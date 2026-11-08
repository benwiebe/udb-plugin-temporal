package boards

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/benwiebe/udb-plugin-library/types"
	"github.com/benwiebe/udb-plugin-temporal/datasources"
	"golang.org/x/image/font"
)

type testDatasource struct {
	data any
	kind string
}

func (d *testDatasource) GetId() string                { return "test" }
func (d *testDatasource) GetName() string              { return "Test" }
func (d *testDatasource) GetType() string              { return d.kind }
func (d *testDatasource) Init(json.RawMessage) error   { return nil }
func (d *testDatasource) GetData() any                 { return d.data }
func (d *testDatasource) Start(context.Context) error  { return nil }
func (d *testDatasource) DataChanged() <-chan struct{} { return nil }

func TestUpcomingEventsTimeTransitionsAndProviderIndependence(t *testing.T) {
	now := time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC)
	ds := &testDatasource{kind: datasources.CalendarEventsType, data: datasources.CalendarEventsData{
		LastUpdated: now,
		Events:      []datasources.CalendarEvent{{ID: "meeting", Title: "Planning", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}},
	}}
	b := NewUpcomingEventsBoard()
	if err := b.Init(json.RawMessage(`{"timezone":"UTC"}`), ds, types.BoardDimensions{Width: 64, Height: 32}); err != nil {
		t.Fatal(err)
	}
	b.now = func() time.Time { return now }
	if lines := b.lines(now); lines[0].text != "Today 09:00" || lines[1].text != "Planning" {
		t.Fatalf("future event: %+v", lines)
	}
	now = now.Add(time.Hour)
	if lines := b.lines(now); lines[0].text != "Now - 10:00" {
		t.Fatalf("ongoing event: %+v", lines)
	}
	now = now.Add(time.Hour)
	if lines := b.lines(now); len(lines) != 1 || lines[0].text != "No events" {
		t.Fatalf("ended event should disappear without a poll: %+v", lines)
	}
	if frame := b.Render(); frame.Duration != time.Second || frame.Img.Bounds() != image.Rect(0, 0, 64, 32) {
		t.Fatalf("bad frame: %+v", frame)
	}
}

func TestUpcomingEventsDistinguishesUnavailableEmptyAndStale(t *testing.T) {
	now := time.Now()
	ds := &testDatasource{kind: datasources.CalendarEventsType, data: datasources.CalendarEventsData{LastError: "offline"}}
	b := NewUpcomingEventsBoard()
	if err := b.Init(nil, ds, types.BoardDimensions{Width: 128, Height: 64}); err != nil {
		t.Fatal(err)
	}
	if lines := b.lines(now); lines[1].text != "unavailable" {
		t.Fatalf("outage: %+v", lines)
	}
	ds.data = datasources.CalendarEventsData{LastUpdated: now}
	if lines := b.lines(now); lines[0].text != "No events" {
		t.Fatalf("empty: %+v", lines)
	}
	ds.data = datasources.CalendarEventsData{LastUpdated: now, LastError: "offline", Events: []datasources.CalendarEvent{{Title: "Last good event", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}}}
	if lines := b.lines(now); !strings.HasPrefix(lines[0].text, "STALE") {
		t.Fatalf("stale: %+v", lines)
	}
}

func TestUpcomingEventsWrapsTitlesAndFitsList(t *testing.T) {
	now := time.Now()
	ds := &testDatasource{kind: datasources.CalendarEventsType, data: datasources.CalendarEventsData{LastUpdated: now, Events: []datasources.CalendarEvent{
		{Title: strings.Repeat("Very long event title ", 20), Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
		{Title: "Second event", Start: now.Add(3 * time.Hour), End: now.Add(4 * time.Hour)},
	}}}
	for _, dims := range []types.BoardDimensions{{Width: 64, Height: 32}, {Width: 128, Height: 64}} {
		b := NewUpcomingEventsBoard()
		if err := b.Init(json.RawMessage(`{"maxEvents":2,"padding":2}`), ds, dims); err != nil {
			t.Fatal(err)
		}
		lines := b.lines(now)
		capacity := (b.inner.Dy() + 1) / (b.face.Metrics().Height.Ceil() + 1)
		if len(lines) > capacity {
			t.Fatalf("%+v: %d lines exceed capacity %d", dims, len(lines), capacity)
		}
		for _, line := range lines {
			if font.MeasureString(b.face, line.text).Ceil() > b.inner.Dx() {
				t.Fatalf("line overflows: %q", line.text)
			}
		}
		if !strings.Contains(lines[len(lines)-1].text, "event") && dims.Width == 128 {
			t.Fatalf("second event missing: %+v", lines)
		}
		assertPadding(t, b.Render().Img, 2, color.Black)
	}
}

func TestUpcomingAllDayAndTimezone(t *testing.T) {
	loc, _ := time.LoadLocation("America/Toronto")
	now := time.Date(2030, 1, 1, 23, 0, 0, 0, loc)
	ds := &testDatasource{kind: datasources.CalendarEventsType, data: datasources.CalendarEventsData{LastUpdated: now}}
	b := NewUpcomingEventsBoard()
	if err := b.Init(json.RawMessage(`{"timezone":"America/Toronto"}`), ds, types.BoardDimensions{Width: 128, Height: 64}); err != nil {
		t.Fatal(err)
	}
	event := datasources.CalendarEvent{Start: time.Date(2030, 1, 2, 0, 0, 0, 0, loc), End: time.Date(2030, 1, 3, 0, 0, 0, 0, loc), AllDay: true}
	if got := b.eventWhen(event, now); got != "Tomorrow All day" {
		t.Fatal(got)
	}
	event.Start = event.Start.AddDate(0, 0, -2)
	if got := b.eventWhen(event, now); got != "Today All day" {
		t.Fatalf("ongoing multi-day event: %s", got)
	}
}

func TestBoardValidation(t *testing.T) {
	now := &testDatasource{kind: datasources.CurrentTimeType, data: time.Now()}
	calendar := &testDatasource{kind: datasources.CalendarEventsType, data: datasources.CalendarEventsData{}}
	for _, factory := range []func() types.Board{
		func() types.Board { return NewDigitalClockBoard() }, func() types.Board { return NewDateBoard() },
		func() types.Board { return NewClockDateBoard() }, func() types.Board { return NewCountdownBoard() }, func() types.Board { return NewUpcomingEventsBoard() },
	} {
		b := factory()
		cfg := json.RawMessage(`{"target":"2030-01-01","padding":0}`)
		ds := types.Datasource(now)
		if b.GetDatasourceType() == datasources.CalendarEventsType {
			ds = calendar
		}
		if err := b.Init(cfg, nil, types.BoardDimensions{Width: 64, Height: 32}); err == nil {
			t.Fatalf("%s accepts nil datasource", b.GetId())
		}
		if err := b.Init(cfg, ds, types.BoardDimensions{}); err == nil {
			t.Fatalf("%s accepts invalid dimensions", b.GetId())
		}
		if err := b.Init(json.RawMessage(`{"target":"2030-01-01","padding":100}`), ds, types.BoardDimensions{Width: 64, Height: 32}); err == nil {
			t.Fatalf("%s accepts padding that hides all text", b.GetId())
		}
	}
	wrong := &testDatasource{kind: datasources.CalendarEventsType, data: "wrong shape"}
	if err := NewUpcomingEventsBoard().Init(nil, wrong, types.BoardDimensions{Width: 64, Height: 32}); err == nil {
		t.Fatal("accepted invalid calendar data shape")
	}
}

func TestCountdownLabelScaleAndLayout(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	ds := &testDatasource{kind: datasources.CurrentTimeType, data: now}
	var heights []int
	for _, ratio := range []float64{0.35, 1.0} {
		b := NewCountdownBoard()
		cfg, _ := json.Marshal(map[string]any{"target": "2030-01-06T00:00:00Z", "label": "Birthday", "labelScale": ratio, "padding": 0})
		if err := b.Init(cfg, ds, types.BoardDimensions{Width: 256, Height: 128}); err != nil {
			t.Fatal(err)
		}
		heights = append(heights, b.cachedLabelHeight)
		if b.cachedCombinedHeight > b.cachedInner.Dy() {
			t.Fatal("label overflows display")
		}
	}
	if heights[0] == heights[1] {
		t.Fatalf("labelScale had no effect: %v", heights)
	}
	b := NewCountdownBoard()
	if err := b.Init(json.RawMessage(`{"target":"2030-01-06","label":"A very long label that must wrap and fit the panel","padding":2}`), ds, types.BoardDimensions{Width: 64, Height: 32}); err != nil {
		t.Fatal(err)
	}
	if b.cachedCombinedHeight > b.cachedInner.Dy() {
		t.Fatal("wrapped countdown overflows")
	}
	if b.cachedLabelHeight > b.cachedCountdownHeight {
		t.Fatal("default label should not be larger than the countdown")
	}
	assertPadding(t, b.Render().Img, 2, color.Black)
}

func TestCountdownRefreshesAtExpiryWithoutSeconds(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 30, 0, time.UTC)
	ds := &testDatasource{kind: datasources.CurrentTimeType, data: now}
	b := NewCountdownBoard()
	if err := b.Init(json.RawMessage(`{"target":"2030-01-01T00:00:45Z","showSeconds":false}`), ds, types.BoardDimensions{Width: 64, Height: 32}); err != nil {
		t.Fatal(err)
	}
	if frame := b.Render(); frame.Duration != 15*time.Second {
		t.Fatalf("expiry would be delayed: %v", frame.Duration)
	}
	ds.data = now.Add(15 * time.Second)
	if got := formatCountdownText(b.target.Sub(ds.data.(time.Time)), false, "NOW"); got != "NOW" {
		t.Fatal(got)
	}
}

func TestDateFitsLongerDatesAndUpdatesAtMidnight(t *testing.T) {
	now := time.Date(2030, 9, 30, 23, 59, 59, 0, time.UTC)
	ds := &testDatasource{kind: datasources.CurrentTimeType, data: now}
	b := NewDateBoard()
	if err := b.Init(json.RawMessage(`{"format":"Mon Jan 2","timezone":"UTC","padding":2}`), ds, types.BoardDimensions{Width: 64, Height: 32}); err != nil {
		t.Fatal(err)
	}
	if font.MeasureString(b.cachedFace, now.Format(b.format)).Ceil() > b.cachedInner.Dx() {
		t.Fatal("date font was sized for a shorter date")
	}
	if frame := b.Render(); frame.Duration != time.Second {
		t.Fatalf("midnight refresh delayed: %v", frame.Duration)
	}
}

func assertPadding(t *testing.T, img image.Image, padding int, background color.Color) {
	t.Helper()
	bg := color.RGBAModel.Convert(background)
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if x < padding || y < padding || x >= bounds.Max.X-padding || y >= bounds.Max.Y-padding {
				if color.RGBAModel.Convert(img.At(x, y)) != bg {
					t.Fatalf("text escaped padding at %d,%d", x, y)
				}
			}
		}
	}
}

func TestTimeBoardsKeepLongFormatsInsidePadding(t *testing.T) {
	now := time.Date(2030, 9, 30, 15, 4, 6, 0, time.UTC)
	ds := &testDatasource{kind: datasources.CurrentTimeType, data: now}
	for _, b := range []types.DynamicBoard{NewDigitalClockBoard(), NewDateBoard(), NewClockDateBoard()} {
		cfg := json.RawMessage(`{"format":"Monday, January 2, 2006","timeFormat":"15:04:05 MST","dateFormat":"Monday, January 2","padding":2,"timezone":"UTC"}`)
		if err := b.Init(cfg, ds, types.BoardDimensions{Width: 64, Height: 32}); err != nil {
			t.Fatal(err)
		}
		assertPadding(t, b.Render().Img, 2, color.Black)
	}
}

func TestAllDayDateDoesNotShiftWithDisplayTimezone(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	ds := &testDatasource{kind: datasources.CalendarEventsType, data: datasources.CalendarEventsData{LastUpdated: now}}
	b := NewUpcomingEventsBoard()
	if err := b.Init(json.RawMessage(`{"timezone":"America/Toronto"}`), ds, types.BoardDimensions{Width: 128, Height: 64}); err != nil {
		t.Fatal(err)
	}
	event := datasources.CalendarEvent{AllDay: true, Start: time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC), End: time.Date(2030, 1, 3, 0, 0, 0, 0, time.UTC)}
	if got := b.eventWhen(event, now); got != "Tomorrow All day" {
		t.Fatalf("all-day date shifted: %s", got)
	}
}
