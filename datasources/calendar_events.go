package datasources

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	ical "github.com/emersion/go-ical"
)

const CalendarEventsType = "benwiebe/udb-plugin-temporal/calendar-events"

type CalendarEvent struct {
	ID          string
	Title       string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Location    string
	Description string
}

type CalendarEventsData struct {
	Events      []CalendarEvent
	LastUpdated time.Time
	Source      string
}

type calendarEventsPollingConfig struct {
	RefreshSeconds *int   `json:"refreshSeconds"`
	LookBackDays   *int   `json:"lookBackDays"`
	LookAheadDays  *int   `json:"lookAheadDays"`
	MaxEvents      *int   `json:"maxEvents"`
	IncludeAllDay  *bool  `json:"includeAllDay"`
	Timezone       string `json:"timezone"`
}

type calendarEventsPollingDatasource struct {
	mu              sync.RWMutex
	data            CalendarEventsData
	changed         chan struct{}
	refreshInterval time.Duration
	lookBack        time.Duration
	lookAhead       time.Duration
	maxEvents       int
	includeAllDay   bool
	location        *time.Location
	source          string
	fetcher         func(context.Context) ([]CalendarEvent, error)
}

func newCalendarEventsPollingDatasource() calendarEventsPollingDatasource {
	return calendarEventsPollingDatasource{
		changed:         make(chan struct{}, 1),
		refreshInterval: 5 * time.Minute,
		lookBack:        24 * time.Hour,
		lookAhead:       30 * 24 * time.Hour,
		maxEvents:       10,
		includeAllDay:   true,
	}
}

func (d *calendarEventsPollingDatasource) initCommon(cfg calendarEventsPollingConfig) error {
	if cfg.RefreshSeconds != nil {
		if *cfg.RefreshSeconds <= 0 {
			return fmt.Errorf("calendar-events: refreshSeconds must be > 0")
		}
		d.refreshInterval = time.Duration(*cfg.RefreshSeconds) * time.Second
	}
	if cfg.LookBackDays != nil {
		if *cfg.LookBackDays < 0 {
			return fmt.Errorf("calendar-events: lookBackDays must be >= 0")
		}
		d.lookBack = time.Duration(*cfg.LookBackDays) * 24 * time.Hour
	}
	if cfg.LookAheadDays != nil {
		if *cfg.LookAheadDays <= 0 {
			return fmt.Errorf("calendar-events: lookAheadDays must be > 0")
		}
		d.lookAhead = time.Duration(*cfg.LookAheadDays) * 24 * time.Hour
	}
	if cfg.MaxEvents != nil {
		if *cfg.MaxEvents <= 0 {
			return fmt.Errorf("calendar-events: maxEvents must be > 0")
		}
		d.maxEvents = *cfg.MaxEvents
	}
	if cfg.IncludeAllDay != nil {
		d.includeAllDay = *cfg.IncludeAllDay
	}
	if cfg.Timezone != "" {
		location, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			return fmt.Errorf("calendar-events: load timezone %q: %w", cfg.Timezone, err)
		}
		d.location = location
	}
	return nil
}

func (d *calendarEventsPollingDatasource) queryWindow() (start, end time.Time) {
	now := time.Now()
	if d.location != nil {
		now = now.In(d.location)
	}
	return now.Add(-d.lookBack), now.Add(d.lookAhead)
}

func (d *calendarEventsPollingDatasource) GetData() any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.data
}

func (d *calendarEventsPollingDatasource) Start(ctx context.Context) error {
	if d.fetcher == nil {
		return fmt.Errorf("calendar-events: datasource fetcher is not configured")
	}
	if err := d.refresh(ctx); err != nil {
		return err
	}

	go func() {
		ticker := time.NewTicker(d.refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = d.refresh(ctx)
			}
		}
	}()

	return nil
}

func (d *calendarEventsPollingDatasource) DataChanged() <-chan struct{} {
	return d.changed
}

func (d *calendarEventsPollingDatasource) refresh(ctx context.Context) error {
	events, err := d.fetcher(ctx)
	if err != nil {
		return err
	}

	events = d.filterUpcoming(events)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Start.Before(events[j].Start)
	})
	if len(events) > d.maxEvents {
		events = events[:d.maxEvents]
	}

	newData := CalendarEventsData{
		Events:      events,
		LastUpdated: time.Now(),
		Source:      d.source,
	}

	d.mu.Lock()
	changed := !calendarEventsEqual(d.data.Events, newData.Events)
	d.data = newData
	d.mu.Unlock()

	if changed {
		select {
		case d.changed <- struct{}{}:
		default:
		}
	}

	return nil
}

func (d *calendarEventsPollingDatasource) filterUpcoming(events []CalendarEvent) []CalendarEvent {
	now := time.Now()
	if d.location != nil {
		now = now.In(d.location)
	}
	cutoff := now.Add(d.lookAhead)

	filtered := make([]CalendarEvent, 0, len(events))
	for _, event := range events {
		if event.End.IsZero() {
			event.End = event.Start
		}
		if event.End.Before(now) {
			continue
		}
		if event.Start.After(cutoff) {
			continue
		}
		if event.AllDay && !d.includeAllDay {
			continue
		}
		filtered = append(filtered, event)
	}

	return filtered
}

func calendarEventsEqual(a, b []CalendarEvent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func convertICalCalendar(cal *ical.Calendar, location *time.Location, windowStart, windowEnd time.Time) []CalendarEvent {
	rawEvents := cal.Events()
	out := make([]CalendarEvent, 0, len(rawEvents))
	for _, rawEvent := range rawEvents {
		events, err := convertICalEvent(rawEvent, location, windowStart, windowEnd)
		if err != nil {
			continue
		}
		out = append(out, events...)
	}
	return out
}

func convertICalEvent(rawEvent ical.Event, location *time.Location, windowStart, windowEnd time.Time) ([]CalendarEvent, error) {
	start, err := rawEvent.DateTimeStart(location)
	if err != nil {
		return nil, err
	}

	end, err := rawEvent.DateTimeEnd(location)
	if err != nil {
		end = start
		if dtStart := rawEvent.Props.Get(ical.PropDateTimeStart); dtStart != nil && dtStart.ValueType() == ical.ValueDate {
			end = start.Add(24 * time.Hour)
		}
	}
	if end.Before(start) {
		end = start
	}

	title, _ := rawEvent.Props.Text(ical.PropSummary)
	if title == "" {
		title = "Untitled"
	}
	locText, _ := rawEvent.Props.Text(ical.PropLocation)
	description, _ := rawEvent.Props.Text(ical.PropDescription)
	uid, _ := rawEvent.Props.Text(ical.PropUID)
	if uid == "" {
		uid = start.Format(time.RFC3339)
	}

	allDay := false
	if dtStart := rawEvent.Props.Get(ical.PropDateTimeStart); dtStart != nil && dtStart.ValueType() == ical.ValueDate {
		allDay = true
	}

	event := CalendarEvent{
		ID:          uid,
		Title:       title,
		Start:       start,
		End:         end,
		AllDay:      allDay,
		Location:    locText,
		Description: description,
	}

	recurrenceSet, err := rawEvent.RecurrenceSet(location)
	if err != nil {
		return nil, err
	}
	if recurrenceSet == nil || windowStart.IsZero() || windowEnd.IsZero() {
		return []CalendarEvent{event}, nil
	}

	duration := event.End.Sub(event.Start)
	recurrenceStart := windowStart.Add(-duration)
	occurrences := recurrenceSet.Between(recurrenceStart, windowEnd, true)
	events := make([]CalendarEvent, 0, len(occurrences))
	for _, occurrence := range occurrences {
		expanded := event
		expanded.Start = occurrence
		expanded.End = occurrence.Add(duration)
		expanded.ID = fmt.Sprintf("%s/%s", event.ID, occurrence.UTC().Format(time.RFC3339))
		events = append(events, expanded)
	}
	return events, nil
}
