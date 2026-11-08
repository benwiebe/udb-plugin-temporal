package datasources

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ical "github.com/emersion/go-ical"
)

func decodeCalendar(t *testing.T, events string) *ical.Calendar {
	t.Helper()
	cal, err := ical.NewDecoder(strings.NewReader("BEGIN:VCALENDAR\r\n" + events + "END:VCALENDAR\r\n")).Decode()
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

func localCalendar(t *testing.T, events string, now time.Time, config map[string]any) *ICalCalendarEventsDatasource {
	t.Helper()
	path := filepath.Join(t.TempDir(), "calendar.ics")
	if err := os.WriteFile(path, []byte("BEGIN:VCALENDAR\r\n"+events+"END:VCALENDAR\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config["path"] = path
	cfg, _ := json.Marshal(config)
	ds := NewICalCalendarEventsDatasource()
	if err := ds.Init(cfg); err != nil {
		t.Fatal(err)
	}
	ds.now = func() time.Time { return now }
	return ds
}

func TestUnchangedCalendarReplenishesEventsAndAdvancesRecurrenceWindow(t *testing.T) {
	now := time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC)
	events := "BEGIN:VEVENT\r\nUID:daily\r\nDTSTART:20300101T090000Z\r\nDTEND:20300101T100000Z\r\nRRULE:FREQ=DAILY;COUNT=10\r\nSUMMARY:Daily\r\nEND:VEVENT\r\n"
	ds := localCalendar(t, events, now, map[string]any{"maxEvents": 1, "lookAheadDays": 1})
	if err := ds.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := ds.GetData().(CalendarEventsData).Events[0]
	cal := ds.calendar
	now = now.AddDate(0, 0, 2)
	ds.now = func() time.Time { return now }
	if err := ds.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	data := ds.GetData().(CalendarEventsData)
	if len(data.Events) != 1 || !data.Events[0].Start.Equal(first.Start.AddDate(0, 0, 2)) {
		t.Fatalf("moving window lost events: %+v", data.Events)
	}
	if ds.calendar != cal {
		t.Fatal("unchanged file should reuse parsed calendar")
	}
	data.Events[0].Title = "mutated"
	if ds.GetData().(CalendarEventsData).Events[0].Title != "Daily" {
		t.Fatal("GetData leaked mutable cache")
	}
}

func TestCalendarOverridesAndCancellation(t *testing.T) {
	cal := decodeCalendar(t, "BEGIN:VEVENT\r\nUID:daily\r\nDTSTART:20300101T090000Z\r\nDTEND:20300101T100000Z\r\nRRULE:FREQ=DAILY;COUNT=3\r\nSUMMARY:Original\r\nEND:VEVENT\r\n"+
		"BEGIN:VEVENT\r\nUID:daily\r\nRECURRENCE-ID:20300102T090000Z\r\nDTSTART:20300102T110000Z\r\nDTEND:20300102T120000Z\r\nSUMMARY:Moved\r\nEND:VEVENT\r\n"+
		"BEGIN:VEVENT\r\nUID:daily\r\nRECURRENCE-ID:20300103T090000Z\r\nSTATUS:CANCELLED\r\nEND:VEVENT\r\n"+
		"BEGIN:VEVENT\r\nUID:cancelled\r\nDTSTART:20300102T150000Z\r\nSTATUS:CANCELLED\r\nEND:VEVENT\r\n")
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	events := convertICalCalendar(cal, time.UTC, start, start.AddDate(0, 0, 4))
	if len(events) != 2 {
		t.Fatalf("expected original and moved occurrence, got %+v", events)
	}
	for _, event := range events {
		if event.Start.Day() == 2 && (event.Title != "Moved" || event.Start.Hour() != 11 || event.ID != "daily/2030-01-02T09:00:00Z") {
			t.Fatalf("bad override: %+v", event)
		}
	}
}

func TestRecurrenceDateLists(t *testing.T) {
	for _, rule := range []string{"", "RRULE:FREQ=DAILY;COUNT=3\r\n"} {
		t.Run(rule, func(t *testing.T) {
			cal := decodeCalendar(t, "BEGIN:VEVENT\r\nUID:dates\r\nDTSTART:20300101T090000Z\r\nDTEND:20300101T100000Z\r\n"+rule+"RDATE:20300104T090000Z,20300105T090000Z\r\nEXDATE:20300102T090000Z,20300103T090000Z\r\nEND:VEVENT\r\n")
			start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			events := convertICalCalendar(cal, time.UTC, start, start.AddDate(0, 0, 6))
			if len(events) != 3 {
				t.Fatalf("expected DTSTART and two RDATEs, got %+v", events)
			}
		})
	}
}

func TestAllDayRecurrenceUsesCalendarDaysAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	cal := decodeCalendar(t, "BEGIN:VEVENT\r\nUID:day\r\nDTSTART;VALUE=DATE:20260307\r\nRRULE:FREQ=DAILY;COUNT=3\r\nEND:VEVENT\r\n")
	start := time.Date(2026, 3, 7, 0, 0, 0, 0, loc)
	events := convertICalCalendar(cal, loc, start, start.AddDate(0, 0, 3))
	if len(events) != 3 {
		t.Fatalf("events: %+v", events)
	}
	for _, event := range events {
		if !event.AllDay || event.Start.Hour() != 0 || event.End.Hour() != 0 || !event.End.Equal(event.Start.AddDate(0, 0, 1)) {
			t.Fatalf("all-day event crossed midnight incorrectly: %+v", event)
		}
	}
	if events[1].End.Sub(events[1].Start) != 23*time.Hour {
		t.Fatal("DST day should last 23 hours")
	}
	cal = decodeCalendar(t, "BEGIN:VEVENT\r\nUID:duration\r\nDTSTART;VALUE=DATE:20260308\r\nDURATION:P1D\r\nEND:VEVENT\r\n")
	events = convertICalCalendar(cal, loc, start, start.AddDate(0, 0, 3))
	if len(events) != 1 || events[0].End.Hour() != 0 || events[0].End.Sub(events[0].Start) != 23*time.Hour {
		t.Fatalf("explicit all-day duration crossed midnight: %+v", events)
	}
}

func TestFailedLocalParseDoesNotAcknowledgeFile(t *testing.T) {
	ds := localCalendar(t, "", time.Now(), map[string]any{})
	if err := os.WriteFile(ds.source, []byte("not a calendar"), 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := ds.fetchEvents(context.Background()); err == nil {
			t.Fatal("invalid unchanged file should still fail")
		}
	}
	if !ds.sourceModTime.IsZero() {
		t.Fatal("failed parse acknowledged file version")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHTTPValidatorsCommitOnlyAfterSuccessfulParse(t *testing.T) {
	valid := "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"
	body, etag := valid, "good"
	ds := NewICalCalendarEventsDatasource()
	if err := ds.Init(json.RawMessage(`{"url":"https://calendar.example/feed.ics"}`)); err != nil {
		t.Fatal(err)
	}
	ds.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("If-None-Match") == etag {
			return &http.Response{StatusCode: 304, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Etag": []string{etag}}}, nil
	})
	if err := ds.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	body, etag = "invalid calendar", "new"
	if err := ds.refresh(context.Background()); err == nil {
		t.Fatal("expected parse failure")
	}
	if ds.etag != "good" {
		t.Fatal("failed parse committed ETag")
	}
	body = valid
	if err := ds.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ds.etag != "new" || ds.GetData().(CalendarEventsData).LastError != "" {
		t.Fatal("did not recover")
	}
}

func TestPollingKeepsLastGoodDataAndSignalsRecovery(t *testing.T) {
	ds := newCalendarEventsPollingDatasource()
	now := time.Now()
	ds.now = func() time.Time { return now }
	fail := false
	ds.fetcher = func(context.Context) ([]CalendarEvent, error) {
		if fail {
			return nil, errors.New("offline")
		}
		return []CalendarEvent{{ID: "event", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}}, nil
	}
	if err := ds.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := ds.GetData().(CalendarEventsData)
	<-ds.DataChanged()
	fail = true
	if err := ds.refresh(context.Background()); err == nil {
		t.Fatal("expected outage")
	}
	after := ds.GetData().(CalendarEventsData)
	if after.LastError == "" || !after.LastUpdated.Equal(before.LastUpdated) || len(after.Events) != 1 {
		t.Fatalf("lost last good data: %+v", after)
	}
	<-ds.DataChanged()
	fail = false
	if err := ds.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ds.DataChanged():
	default:
		t.Fatal("recovery must signal even with unchanged events")
	}
}

func TestStartRetriesInitialOutageAndStopsWithContext(t *testing.T) {
	ds := newCalendarEventsPollingDatasource()
	ds.refreshInterval = 10 * time.Millisecond
	attempts := make(chan struct{}, 10)
	ds.fetcher = func(ctx context.Context) ([]CalendarEvent, error) {
		attempts <- struct{}{}
		return nil, errors.New("offline")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ds.Start(ctx); err != nil {
		t.Fatalf("outage prevented startup: %v", err)
	}
	for range 2 {
		select {
		case <-attempts:
		case <-time.After(time.Second):
			t.Fatal("no retry")
		}
	}
	cancel()
	// Check cancellation directly, without waiting for another ticker iteration.
	if err := ds.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start: %v", err)
	}
}

func TestCalDAVRequestsExpansionAndBasicAuth(t *testing.T) {
	var report []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "alice" || password != "secret" {
			t.Error("missing basic auth")
		}
		if r.Method != "REPORT" || r.URL.Path != "/personal/" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		report, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		var data bytes.Buffer
		xml.EscapeText(&data, []byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:test\r\nDTSTART:20300101T090000Z\r\nDTEND:20300101T100000Z\r\nSUMMARY:Meeting\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"))
		fmt.Fprintf(w, `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>/personal/test.ics</d:href><d:propstat><d:prop><c:calendar-data>%s</c:calendar-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`, data.String())
	}))
	defer server.Close()
	ds := NewCalDAVCalendarEventsDatasource()
	cfg, _ := json.Marshal(map[string]any{"endpoint": server.URL, "calendarPath": "/personal/", "username": "alice", "password": "secret", "lookAheadDays": 3})
	if err := ds.Init(cfg); err != nil {
		t.Fatal(err)
	}
	ds.now = func() time.Time { return time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ds.Start(ctx); err != nil {
		t.Fatal(err)
	}
	var query struct {
		Prop struct {
			Data struct {
				Expand *struct {
					Start string `xml:"start,attr"`
					End   string `xml:"end,attr"`
				} `xml:"expand"`
			} `xml:"calendar-data"`
		} `xml:"prop"`
		Filter struct {
			Comp struct {
				Child struct {
					Range *struct {
						Start string `xml:"start,attr"`
						End   string `xml:"end,attr"`
					} `xml:"time-range"`
				} `xml:"comp-filter"`
			} `xml:"comp-filter"`
		} `xml:"filter"`
	}
	if err := xml.Unmarshal(report, &query); err != nil {
		t.Fatal(err)
	}
	expand, window := query.Prop.Data.Expand, query.Filter.Comp.Child.Range
	if expand == nil || window == nil || expand.Start != "20291231T080000Z" || expand.End != "20300104T080000Z" || window.Start != expand.Start || window.End != expand.End {
		t.Fatalf("missing or misplaced bounded expansion: %s", report)
	}
	if data := ds.GetData().(CalendarEventsData); len(data.Events) != 1 || data.Events[0].Title != "Meeting" {
		t.Fatalf("bad CalDAV response: %+v", data)
	}
}

func TestCalDAVDoesNotFallbackFromMissingCalendarName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var prop string
		switch r.URL.Path {
		case "/":
			prop = `<d:current-user-principal><d:href>/principal/</d:href></d:current-user-principal>`
		case "/principal/":
			prop = `<c:calendar-home-set><d:href>/home/</d:href></c:calendar-home-set>`
		case "/home/":
			prop = `<d:resourcetype><d:collection/><c:calendar/></d:resourcetype><d:displayname>Other</d:displayname><c:supported-calendar-component-set><c:comp name="VEVENT"/></c:supported-calendar-component-set>`
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		fmt.Fprintf(w, `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>/home/other/</d:href><d:propstat><d:prop>%s</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`, prop)
	}))
	defer server.Close()
	ds := NewCalDAVCalendarEventsDatasource()
	cfg, _ := json.Marshal(map[string]any{"endpoint": server.URL, "calendarName": "Missing"})
	if err := ds.Init(cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ds.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if ds.calendarPath != "" || !strings.Contains(ds.GetData().(CalendarEventsData).LastError, "Missing") {
		t.Fatal("silently selected a different calendar")
	}
}
