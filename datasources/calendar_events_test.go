package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestICalCalendarEventsDatasource_ParsesTimedAndAllDayEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calendar.ics")
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:event-1\r\nDTSTART:20300102T150405Z\r\nDTEND:20300102T160405Z\r\nSUMMARY:Timed Event\r\nLOCATION:Office\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:event-2\r\nDTSTART;VALUE=DATE:20300103\r\nSUMMARY:All Day Event\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if err := os.WriteFile(path, []byte(ics), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ds := NewICalCalendarEventsDatasource()
	cfg, _ := json.Marshal(map[string]any{"path": path})
	if err := ds.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}

	events, err := ds.fetchEvents(context.Background())
	if err != nil {
		t.Fatalf("fetchEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != "event-1" || events[0].Title != "Timed Event" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].ID != "event-2" || !events[1].AllDay {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
}

func TestICalCalendarEventsDatasource_RefreshFiltersAndSignals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calendar.ics")

	now := time.Now().UTC()
	future := now.Add(2 * time.Hour).Format("20060102T150405Z")
	futureEnd := now.Add(3 * time.Hour).Format("20060102T150405Z")
	tooFar := now.Add(40 * 24 * time.Hour).Format("20060102T150405Z")
	tooFarEnd := now.Add(40*24*time.Hour + time.Hour).Format("20060102T150405Z")

	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\nUID:soon\r\nDTSTART:" + future + "\r\nDTEND:" + futureEnd + "\r\nSUMMARY:Soon\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:later\r\nDTSTART:" + tooFar + "\r\nDTEND:" + tooFarEnd + "\r\nSUMMARY:Later\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	if err := os.WriteFile(path, []byte(ics), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ds := NewICalCalendarEventsDatasource()
	cfg, _ := json.Marshal(map[string]any{
		"path":           path,
		"lookAheadDays":  7,
		"refreshSeconds": 60,
		"maxEvents":      5,
	})
	if err := ds.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := ds.refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	data := ds.GetData().(CalendarEventsData)
	if len(data.Events) != 1 {
		t.Fatalf("expected 1 upcoming event after filtering, got %d", len(data.Events))
	}
	if data.Events[0].ID != "soon" {
		t.Fatalf("expected event 'soon', got %+v", data.Events[0])
	}

	select {
	case <-ds.DataChanged():
	default:
		t.Fatal("expected refresh to signal DataChanged")
	}
}

func TestICalCalendarEventsDatasource_ExpandsRecurringEventsInWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calendar.ics")

	now := time.Now().UTC()
	original := now.Add(-14 * 24 * time.Hour).Format("20060102T150405Z")
	originalEnd := now.Add(-14*24*time.Hour + time.Hour).Format("20060102T150405Z")
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\nUID:standup\r\nDTSTART:" + original + "\r\nDTEND:" + originalEnd + "\r\nRRULE:FREQ=DAILY;COUNT=30\r\nSUMMARY:Standup\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	if err := os.WriteFile(path, []byte(ics), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ds := NewICalCalendarEventsDatasource()
	cfg, _ := json.Marshal(map[string]any{
		"path":          path,
		"lookBackDays":  1,
		"lookAheadDays": 3,
		"maxEvents":     10,
	})
	if err := ds.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := ds.refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	data := ds.GetData().(CalendarEventsData)
	if len(data.Events) == 0 {
		t.Fatal("expected recurring events in query window")
	}
	for _, event := range data.Events {
		if event.ID == "standup" {
			t.Fatalf("expected expanded occurrence ID, got base ID: %+v", event)
		}
		if event.Title != "Standup" {
			t.Fatalf("unexpected recurring event title: %+v", event)
		}
	}
}

func TestICalCalendarEventsDatasource_LocalSourceSkipsUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calendar.ics")
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:event-1\r\nDTSTART:20300102T150405Z\r\nDTEND:20300102T160405Z\r\nSUMMARY:Timed Event\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if err := os.WriteFile(path, []byte(ics), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ds := NewICalCalendarEventsDatasource()
	cfg, _ := json.Marshal(map[string]any{"path": path})
	if err := ds.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if _, err := ds.fetchEvents(context.Background()); err != nil {
		t.Fatalf("first fetchEvents: %v", err)
	}

	if _, modified, err := ds.readSource(context.Background()); err != nil {
		t.Fatalf("second readSource: %v", err)
	} else if modified {
		t.Fatal("expected unchanged file to skip reload")
	}
}

func TestICalCalendarEventsDatasource_RespectsMaxBytesForHTTP(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" + strings.Repeat("X", 256) + "\r\nEND:VCALENDAR\r\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ics))
	}))
	defer server.Close()

	ds := NewICalCalendarEventsDatasource()
	cfg, _ := json.Marshal(map[string]any{
		"url":      server.URL,
		"maxBytes": 64,
	})
	if err := ds.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if _, err := ds.fetchEvents(context.Background()); err == nil {
		t.Fatal("expected oversized payload to fail")
	}
}

func TestNormalizeCalendarURL(t *testing.T) {
	if got := normalizeCalendarURL("webcal://example.com/calendar.ics"); got != "https://example.com/calendar.ics" {
		t.Fatalf("unexpected webcal normalization: %q", got)
	}
	if got := normalizeCalendarURL("webcals://example.com/calendar.ics"); got != "https://example.com/calendar.ics" {
		t.Fatalf("unexpected webcals normalization: %q", got)
	}
	if got := normalizeCalendarURL("https://example.com/calendar.ics"); got != "https://example.com/calendar.ics" {
		t.Fatalf("unexpected https normalization: %q", got)
	}
}

func TestCalDAVCalendarEventsDatasource_InitValidation(t *testing.T) {
	ds := NewCalDAVCalendarEventsDatasource()
	if err := ds.Init(json.RawMessage(`{"endpoint":"https://example.com/caldav"}`)); err == nil {
		t.Fatal("expected missing calendar selector to fail init")
	}

	if err := ds.Init(json.RawMessage(`{"endpoint":"https://example.com/caldav","calendarName":"Personal","lookBackDays":-1}`)); err == nil {
		t.Fatal("expected negative lookBackDays to fail init")
	}
}

func TestSupportsEventComponent(t *testing.T) {
	if !supportsEventComponent(nil) {
		t.Fatal("expected empty component set to be treated as VEVENT-capable")
	}
	if !supportsEventComponent([]string{"VTODO", "VEVENT"}) {
		t.Fatal("expected VEVENT support to be detected")
	}
	if supportsEventComponent([]string{"VTODO"}) {
		t.Fatal("did not expect non-VEVENT component set to match")
	}
}
