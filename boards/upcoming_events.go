package boards

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sort"
	"strings"
	"time"

	"github.com/benwiebe/udb-plugin-library/fonts"
	"github.com/benwiebe/udb-plugin-library/types"
	"github.com/benwiebe/udb-plugin-temporal/datasources"
	"github.com/benwiebe/udb-plugin-temporal/helpers"
	"golang.org/x/image/font"
)

// UpcomingEventsBoard displays upcoming calendar events provided by a CalendarEventsData datasource.
// It renders from memory-cached data, performing no network requests during rendering.
//
// Configuration fields (via JSON config):
//   - maxEvents: maximum number of upcoming events to render (default 1)
//   - timezone: IANA timezone identifier for formatting event times (e.g., "America/Toronto"; default system local)
//   - timeFormat: Go time layout string for event start/end times (default "15:04")
//   - dateFormat: Go time layout string for event dates when beyond tomorrow (default "Jan 2")
//   - emptyText: message displayed when no events are scheduled (default "No events")
//   - showLocation: whether to render the event location on its own line if space allows (default false)
//   - padding: padding in pixels on all sides (default 10% of width/height)
//   - fontStyle: pixel font style ("auto", "dotMatrix", "scoreboard", "condensed", "outlined", "silkscreenBold", "pixelifySansBold")
//   - fontScale: integer pixel scaling multiplier (default 0, automatic fit)
//   - fillScreen: when true, removes padding for edge-to-edge rendering (default false)
//   - colour: hex colour for event title text (default "#FFFFFF")
//   - timeColour: hex colour for event time and location text (default "#64B4FF")
//   - backgroundColour: hex colour for the background (default "#000000")
type UpcomingEventsBoard struct {
	datasource                           types.Datasource
	dimensions                           types.BoardDimensions
	inner                                image.Rectangle
	location                             *time.Location
	maxEvents                            int
	timeFormat, dateFormat, emptyText    string
	showLocation                         bool
	padding                              *int
	colour, timeColour, backgroundColour color.Color
	fontOptions                          fontOptions
	face                                 font.Face
	now                                  func() time.Time
}

// NewUpcomingEventsBoard constructs an uninitialized UpcomingEventsBoard.
func NewUpcomingEventsBoard() *UpcomingEventsBoard                             { return &UpcomingEventsBoard{now: time.Now} }

// GetId returns the board identifier ("upcoming-events").
func (b *UpcomingEventsBoard) GetId() string                                   { return "upcoming-events" }

// GetName returns the display name ("Upcoming Events").
func (b *UpcomingEventsBoard) GetName() string                                 { return "Upcoming Events" }

// GetSupportedDimensions returns supported dimensions (empty slice allows dynamic sizing).
func (b *UpcomingEventsBoard) GetSupportedDimensions() []types.BoardDimensions { return nil }

// GetType returns the board type (dynamic animation frame generation).
func (b *UpcomingEventsBoard) GetType() types.BoardType                        { return types.BoardTypeDynamic }

// GetDatasourceType returns the expected datasource type (CalendarEventsType).
func (b *UpcomingEventsBoard) GetDatasourceType() string                       { return datasources.CalendarEventsType }

// Init parses configuration, validates display dimensions, binds the calendar datasource,
// and resolves font sizing for event titles and timestamps.
func (b *UpcomingEventsBoard) Init(cfg json.RawMessage, ds types.Datasource, dims types.BoardDimensions) error {
	if ds == nil || ds.GetType() != datasources.CalendarEventsType {
		return fmt.Errorf("upcoming-events: expected a calendar-events datasource")
	}
	if _, ok := ds.GetData().(datasources.CalendarEventsData); !ok {
		return fmt.Errorf("upcoming-events: datasource must return CalendarEventsData")
	}
	b.datasource, b.dimensions, b.location = ds, dims, time.Local
	b.maxEvents, b.timeFormat, b.dateFormat, b.emptyText = 1, "15:04", "Jan 2", "No events"
	b.colour, b.timeColour, b.backgroundColour = color.White, color.RGBA{R: 100, G: 180, B: 255, A: 255}, color.Black
	var parsed struct {
		MaxEvents        *int   `json:"maxEvents"`
		Timezone         string `json:"timezone"`
		TimeFormat       string `json:"timeFormat"`
		DateFormat       string `json:"dateFormat"`
		EmptyText        string `json:"emptyText"`
		ShowLocation     *bool  `json:"showLocation"`
		Padding          *int   `json:"padding"`
		FontStyle        string `json:"fontStyle"`
		FontScale        *int   `json:"fontScale"`
		FillScreen       *bool  `json:"fillScreen"`
		Colour           string `json:"colour"`
		TimeColour       string `json:"timeColour"`
		BackgroundColour string `json:"backgroundColour"`
	}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return err
		}
	}
	if parsed.MaxEvents != nil {
		if *parsed.MaxEvents <= 0 {
			return fmt.Errorf("upcoming-events: maxEvents must be > 0")
		}
		b.maxEvents = *parsed.MaxEvents
	}
	if parsed.Timezone != "" {
		loc, err := time.LoadLocation(parsed.Timezone)
		if err != nil {
			return fmt.Errorf("upcoming-events: timezone: %w", err)
		}
		b.location = loc
	}
	if parsed.TimeFormat != "" {
		b.timeFormat = parsed.TimeFormat
	}
	if parsed.DateFormat != "" {
		b.dateFormat = parsed.DateFormat
	}
	if parsed.EmptyText != "" {
		b.emptyText = parsed.EmptyText
	}
	if parsed.ShowLocation != nil {
		b.showLocation = *parsed.ShowLocation
	}
	b.padding = parsed.Padding
	fontOpts, err := parseFontOptions(parsed.FontStyle, parsed.FontScale, "upcoming-events")
	if err != nil {
		return err
	}
	b.fontOptions = fontOpts
	if parsed.FillScreen != nil {
		setFillScreen(&b.padding, *parsed.FillScreen)
	}
	for _, setting := range []struct {
		value  string
		target *color.Color
	}{{parsed.Colour, &b.colour}, {parsed.TimeColour, &b.timeColour}, {parsed.BackgroundColour, &b.backgroundColour}} {
		if setting.value != "" {
			c, err := helpers.ParseHexColour(setting.value)
			if err != nil {
				return err
			}
			*setting.target = c
		}
	}
	if err := validateBoardDimensions(dims, b.padding); err != nil {
		return fmt.Errorf("upcoming-events: %w", err)
	}
	b.inner = resolveInnerRect(dims, b.padding)
	if b.inner.Dy() < 11 {
		return fmt.Errorf("upcoming-events: display and padding must leave at least 11 pixels of height")
	}
	if b.face != nil {
		b.face.Close()
	}
	if hasFontOptions(b.fontOptions) {
		maxFontHeight := (b.inner.Dy()+1)/2 - 1
		b.face, err = configuredTextFace("Event", b.inner.Dx(), maxFontHeight, b.fontOptions)
		if err != nil {
			return fmt.Errorf("upcoming-events: %w", err)
		}
	} else {
		style := fonts.PixelFontDotMatrix
		if b.inner.Dx() < 80 || b.inner.Dy() < 32 {
			style = fonts.PixelFontCondensed
		}
		b.face = fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{Style: style, Scale: 1, LetterSpacing: -1})
	}
	if (b.inner.Dy()+1)/(b.face.Metrics().Height.Ceil()+1) < 2 {
		b.face.Close()
		b.face = nil
		return fmt.Errorf("upcoming-events: font settings leave insufficient height for an event and title")
	}
	return nil
}

type calendarTextLine struct {
	text   string
	colour color.Color
}

func (b *UpcomingEventsBoard) lines(now time.Time) []calendarTextLine {
	data, ok := b.datasource.GetData().(datasources.CalendarEventsData)
	if !ok || data.LastUpdated.IsZero() {
		return []calendarTextLine{{"Calendar", b.colour}, {"unavailable", b.timeColour}}
	}
	var events []datasources.CalendarEvent
	for _, event := range data.Events {
		end := event.End
		if end.IsZero() {
			end = event.Start
		}
		if end.After(now) {
			events = append(events, event)
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Start.Before(events[j].Start) })
	if len(events) == 0 {
		if data.LastError != "" {
			return []calendarTextLine{{"Calendar", b.colour}, {"unavailable", b.timeColour}}
		}
		return []calendarTextLine{{b.emptyText, b.colour}}
	}
	lineHeight := b.face.Metrics().Height.Ceil() + 1
	capacity := (b.inner.Dy() + 1) / lineHeight
	limit := b.maxEvents
	// Always leave room for an event's date/time and at least one title line.
	if maxFit := capacity / 2; maxFit < limit {
		limit = maxFit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > len(events) {
		limit = len(events)
	}
	var lines []calendarTextLine
	for i, event := range events[:limit] {
		when := b.eventWhen(event, now)
		if i == 0 && data.LastError != "" {
			when = "STALE " + when
		}
		lines = append(lines, calendarTextLine{truncateText(b.face, when, b.inner.Dx()), b.timeColour})
		title := strings.TrimSpace(event.Title)
		if title == "" {
			title = "Untitled"
		}
		titleLines := wrapTextLines(b.face, title, b.inner.Dx())
		remainingEvents := limit - i - 1
		budget := capacity - len(lines) - remainingEvents*2
		if b.showLocation && event.Location != "" && budget > 1 {
			budget--
		}
		if budget < 1 {
			budget = 1
		}
		if len(titleLines) > budget {
			titleLines = titleLines[:budget]
			titleLines[budget-1] = truncateText(b.face, titleLines[budget-1]+"...", b.inner.Dx())
		}
		for _, titleLine := range titleLines {
			lines = append(lines, calendarTextLine{titleLine, b.colour})
		}
		if b.showLocation && event.Location != "" && len(lines) < capacity-remainingEvents*2 {
			lines = append(lines, calendarTextLine{truncateText(b.face, event.Location, b.inner.Dx()), b.timeColour})
		}
	}
	return lines
}

func (b *UpcomingEventsBoard) eventWhen(event datasources.CalendarEvent, now time.Time) string {
	start, today := event.Start.In(b.location), now.In(b.location)
	// All-day dates are calendar dates, not instants to shift to another zone.
	if event.AllDay {
		start, today = event.Start, now.In(event.Start.Location())
	}
	date := start.Format(b.dateFormat)
	if start.Year() == today.Year() && start.YearDay() == today.YearDay() {
		date = "Today"
	} else if tomorrow := today.AddDate(0, 0, 1); start.Year() == tomorrow.Year() && start.YearDay() == tomorrow.YearDay() {
		date = "Tomorrow"
	}
	if event.AllDay {
		if !event.Start.After(now) && event.End.After(now) {
			date = "Today"
		}
		return date + " All day"
	}
	if !event.Start.After(now) && event.End.After(now) {
		return "Now - " + event.End.In(b.location).Format(b.timeFormat)
	}
	when := date + " " + start.Format(b.timeFormat)
	if font.MeasureString(b.face, when).Ceil() > b.inner.Dx() && date == "Tomorrow" {
		when = "Tmr " + start.Format(b.timeFormat)
	}
	return when
}

// Render draws the current slice of active/upcoming events into an animation frame.
// Frames refresh every second to update ongoing event time badges and countdowns.
func (b *UpcomingEventsBoard) Render() types.AnimationFrame {
	img := image.NewRGBA(image.Rect(0, 0, b.dimensions.Width, b.dimensions.Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(b.backgroundColour), image.Point{}, draw.Src)
	// Clip to the content rectangle, including on unusually small panels.
	content := img.SubImage(b.inner).(draw.Image)
	metrics := b.face.Metrics()
	y := b.inner.Min.Y + metrics.Ascent.Ceil()
	for _, line := range b.lines(b.now()) {
		if y+metrics.Descent.Ceil() > b.inner.Max.Y {
			break
		}
		drawTextLine(content, b.face, image.NewUniform(line.colour), truncateText(b.face, line.text, b.inner.Dx()), b.inner.Min.X, y)
		y += metrics.Height.Ceil() + 1
	}
	return types.AnimationFrame{Img: img, Duration: time.Second}
}
