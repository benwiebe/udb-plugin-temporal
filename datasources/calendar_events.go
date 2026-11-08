package datasources

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
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
	LastError   string // Latest refresh error; empty after a successful refresh.
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
	now             func() time.Time
}

func newCalendarEventsPollingDatasource() calendarEventsPollingDatasource {
	return calendarEventsPollingDatasource{
		changed:         make(chan struct{}, 1),
		refreshInterval: 5 * time.Minute,
		lookBack:        24 * time.Hour,
		lookAhead:       30 * 24 * time.Hour,
		maxEvents:       10,
		includeAllDay:   true,
		location:        time.Local,
		now:             time.Now,
	}
}

func (d *calendarEventsPollingDatasource) initCommon(cfg calendarEventsPollingConfig) error {
	if cfg.RefreshSeconds != nil {
		if *cfg.RefreshSeconds <= 0 || int64(*cfg.RefreshSeconds) > int64((1<<63-1)/time.Second) {
			return fmt.Errorf("calendar-events: refreshSeconds must be > 0 and fit in a time.Duration")
		}
		d.refreshInterval = time.Duration(*cfg.RefreshSeconds) * time.Second
	}
	if cfg.LookBackDays != nil {
		if *cfg.LookBackDays < 0 || int64(*cfg.LookBackDays) > int64((1<<63-1)/(24*time.Hour)) {
			return fmt.Errorf("calendar-events: lookBackDays must be >= 0 and fit in a time.Duration")
		}
		d.lookBack = time.Duration(*cfg.LookBackDays) * 24 * time.Hour
	}
	if cfg.LookAheadDays != nil {
		if *cfg.LookAheadDays <= 0 || int64(*cfg.LookAheadDays) > int64((1<<63-1)/(24*time.Hour)) {
			return fmt.Errorf("calendar-events: lookAheadDays must be > 0 and fit in a time.Duration")
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
	now := d.now()
	if d.location != nil {
		now = now.In(d.location)
	}
	return now.Add(-d.lookBack), now.Add(d.lookAhead)
}

func (d *calendarEventsPollingDatasource) GetData() any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	data := d.data
	data.Events = append([]CalendarEvent(nil), data.Events...)
	return data
}

func (d *calendarEventsPollingDatasource) Start(ctx context.Context) error {
	if d.fetcher == nil {
		return fmt.Errorf("calendar-events: datasource fetcher is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// A temporary outage must not prevent UDB from wiring this datasource.
	_ = d.refresh(ctx)
	if err := ctx.Err(); err != nil {
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
		d.mu.Lock()
		changed := d.data.LastError != err.Error()
		d.data.LastError = err.Error()
		d.data.Source = d.source
		d.mu.Unlock()
		if changed {
			slog.Warn("calendar refresh failed; will retry", "err", err)
			d.signalChanged()
		}
		return err
	}

	events = d.filterUpcoming(events)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Start.Equal(events[j].Start) {
			return events[i].ID < events[j].ID
		}
		return events[i].Start.Before(events[j].Start)
	})
	if len(events) > d.maxEvents {
		events = events[:d.maxEvents]
	}

	newData := CalendarEventsData{
		Events:      events,
		LastUpdated: d.now(),
		Source:      d.source,
	}

	d.mu.Lock()
	changed := d.data.LastUpdated.IsZero() || d.data.LastError != "" || !calendarEventsEqual(d.data.Events, newData.Events)
	d.data = newData
	d.mu.Unlock()

	if changed {
		d.signalChanged()
	}

	return nil
}

func (d *calendarEventsPollingDatasource) signalChanged() {
	select {
	case d.changed <- struct{}{}:
	default:
	}
}

func (d *calendarEventsPollingDatasource) filterUpcoming(events []CalendarEvent) []CalendarEvent {
	now := d.now()
	if d.location != nil {
		now = now.In(d.location)
	}
	cutoff := now.Add(d.lookAhead)

	filtered := make([]CalendarEvent, 0, len(events))
	for _, event := range events {
		if event.End.IsZero() {
			event.End = event.Start
		}
		if !event.End.After(now) {
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
		if !a[i].Start.Equal(b[i].Start) || !a[i].End.Equal(b[i].End) ||
			a[i].ID != b[i].ID || a[i].Title != b[i].Title || a[i].AllDay != b[i].AllDay ||
			a[i].Location != b[i].Location || a[i].Description != b[i].Description {
			return false
		}
	}
	return true
}

func convertICalCalendar(cal *ical.Calendar, location *time.Location, windowStart, windowEnd time.Time) []CalendarEvent {
	rawEvents := cal.Events()
	// Detached RECURRENCE-ID components replace (or cancel) a single occurrence.
	overrides := make(map[string]ical.Event)
	cancelledSeries := make(map[string]bool)
	for _, raw := range rawEvents {
		uid, _ := raw.Props.Text(ical.PropUID)
		if prop := raw.Props.Get(ical.PropRecurrenceID); prop != nil {
			original, err := prop.DateTime(location)
			if err == nil {
				overrides[occurrenceID(uid, original)] = raw
			}
		} else if eventCancelled(raw) {
			cancelledSeries[uid] = true
		}
	}
	out := make([]CalendarEvent, 0, len(rawEvents))
	for _, rawEvent := range rawEvents {
		uid, _ := rawEvent.Props.Text(ical.PropUID)
		if rawEvent.Props.Get(ical.PropRecurrenceID) != nil || cancelledSeries[uid] {
			continue
		}
		events, err := convertICalEvent(rawEvent, location, windowStart, windowEnd)
		if err != nil {
			slog.Warn("skipping invalid calendar event", "uid", uid, "err", err)
			continue
		}
		for _, event := range events {
			if _, overridden := overrides[occurrenceID(uid, event.Start)]; !overridden {
				out = append(out, event)
			}
		}
	}
	for id, raw := range overrides {
		uid, _ := raw.Props.Text(ical.PropUID)
		if eventCancelled(raw) || cancelledSeries[uid] {
			continue
		}
		events, err := convertICalEvent(raw, location, windowStart, windowEnd)
		if err != nil {
			slog.Warn("skipping invalid calendar override", "uid", uid, "err", err)
			continue
		}
		for _, event := range events {
			event.ID = id
			out = append(out, event)
		}
	}
	return out
}

func eventCancelled(event ical.Event) bool {
	status, _ := event.Props.Text(ical.PropStatus)
	return strings.EqualFold(status, "CANCELLED")
}

func occurrenceID(uid string, start time.Time) string {
	return uid + "/" + start.UTC().Format(time.RFC3339)
}

func convertICalEvent(rawEvent ical.Event, location *time.Location, windowStart, windowEnd time.Time) ([]CalendarEvent, error) {
	if eventCancelled(rawEvent) {
		return nil, nil
	}
	start, err := rawEvent.DateTimeStart(location)
	if err != nil || start.IsZero() {
		if err == nil {
			err = fmt.Errorf("DTSTART is required")
		}
		return nil, err
	}

	end, err := rawEvent.DateTimeEnd(location)
	if err != nil {
		return nil, err
	}
	if end.Before(start) {
		return nil, fmt.Errorf("DTEND precedes DTSTART")
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
		if rawEvent.Props.Get(ical.PropDateTimeEnd) == nil && rawEvent.Props.Get(ical.PropDuration) == nil {
			end = start.AddDate(0, 0, 1)
		} else if rawEvent.Props.Get(ical.PropDateTimeEnd) == nil {
			duration, err := rawEvent.Props.Get(ical.PropDuration).Duration()
			if err != nil || duration <= 0 || duration%(24*time.Hour) != 0 {
				return nil, fmt.Errorf("all-day DURATION must be a positive whole number of days")
			}
			end = start.AddDate(0, 0, int(duration/(24*time.Hour)))
		}
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

	recurrenceSet, err := calendarRecurrenceSet(rawEvent, start, location)
	if err != nil {
		return nil, err
	}
	if recurrenceSet == nil || windowStart.IsZero() || windowEnd.IsZero() {
		return []CalendarEvent{event}, nil
	}

	duration := event.End.Sub(event.Start)
	dayCount := calendarDayCount(event.Start, event.End)
	recurrenceStart := windowStart.Add(-duration)
	if allDay {
		recurrenceStart = windowStart.AddDate(0, 0, -dayCount)
	}
	occurrences := recurrenceSet.Between(recurrenceStart, windowEnd, true)
	events := make([]CalendarEvent, 0, len(occurrences))
	for _, occurrence := range occurrences {
		expanded := event
		expanded.Start = occurrence
		expanded.End = occurrence.Add(duration)
		if allDay {
			expanded.End = occurrence.AddDate(0, 0, dayCount)
		}
		expanded.ID = occurrenceID(event.ID, occurrence)
		events = append(events, expanded)
	}
	return events, nil
}

func calendarDayCount(start, end time.Time) int {
	end = end.In(start.Location())
	s := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	e := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	return int(e.Sub(s) / (24 * time.Hour))
}

// go-ical's RecurrenceSet does not handle comma-separated date lists or RDATE
// without RRULE. Build the set explicitly while preserving DTSTART's timezone.
func calendarRecurrenceSet(event ical.Event, start time.Time, location *time.Location) (*rrule.Set, error) {
	ruleProp := event.Props.Get(ical.PropRecurrenceRule)
	if ruleProp == nil && len(event.Props[ical.PropRecurrenceDates]) == 0 && len(event.Props[ical.PropExceptionDates]) == 0 {
		return nil, nil
	}
	set := &rrule.Set{}
	set.DTStart(start)
	set.RDate(start)
	if ruleProp != nil {
		options, err := rrule.StrToROptionInLocation(ruleProp.Value, start.Location())
		if err != nil {
			return nil, err
		}
		options.Dtstart = start
		rule, err := rrule.NewRRule(*options)
		if err != nil {
			return nil, err
		}
		set.RRule(rule)
	}
	for _, name := range []string{ical.PropRecurrenceDates, ical.PropExceptionDates} {
		for _, prop := range event.Props[name] {
			for _, value := range strings.Split(prop.Value, ",") {
				dateProp := prop
				dateProp.Value = value
				date, err := dateProp.DateTime(location)
				if err != nil {
					return nil, err
				}
				if name == ical.PropRecurrenceDates {
					set.RDate(date)
				} else {
					set.ExDate(date)
				}
			}
		}
	}
	return set, nil
}
