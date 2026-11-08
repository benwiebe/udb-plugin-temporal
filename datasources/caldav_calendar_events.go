package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	webdav "github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
)

type CalDAVCalendarEventsDatasource struct {
	calendarEventsPollingDatasource
	client       *caldav.Client
	calendarPath string
	calendarName string
	endpoint     string
	username     string
	password     string
}

func NewCalDAVCalendarEventsDatasource() *CalDAVCalendarEventsDatasource {
	return &CalDAVCalendarEventsDatasource{
		calendarEventsPollingDatasource: newCalendarEventsPollingDatasource(),
	}
}

func (d *CalDAVCalendarEventsDatasource) GetId() string {
	return "benwiebe/udb-plugin-temporal/caldav-calendar-events"
}

func (d *CalDAVCalendarEventsDatasource) GetName() string {
	return "CalDAV Calendar Events"
}

func (d *CalDAVCalendarEventsDatasource) GetType() string {
	return CalendarEventsType
}

func (d *CalDAVCalendarEventsDatasource) Init(cfg json.RawMessage) error {
	var parsed struct {
		Endpoint     string `json:"endpoint"`
		CalendarPath string `json:"calendarPath"`
		CalendarName string `json:"calendarName"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		calendarEventsPollingConfig
	}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return err
		}
	}

	if parsed.Endpoint == "" {
		return fmt.Errorf("caldav-calendar-events: endpoint is required")
	}
	u, err := url.Parse(parsed.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("caldav-calendar-events: endpoint must be an HTTP(S) URL")
	}
	if parsed.CalendarPath == "" && parsed.CalendarName == "" {
		return fmt.Errorf("caldav-calendar-events: calendarPath or calendarName is required")
	}

	if err := d.initCommon(parsed.calendarEventsPollingConfig); err != nil {
		return err
	}

	d.endpoint = parsed.Endpoint
	d.calendarPath = parsed.CalendarPath
	d.calendarName = parsed.CalendarName
	d.username = parsed.Username
	d.password = parsed.Password
	d.source = parsed.Endpoint
	d.fetcher = d.fetchEvents
	return nil
}

func (d *CalDAVCalendarEventsDatasource) Start(ctx context.Context) error {
	httpClient := webdav.HTTPClient(&http.Client{Timeout: 15 * time.Second})
	if d.username != "" || d.password != "" {
		httpClient = webdav.HTTPClientWithBasicAuth(httpClient, d.username, d.password)
	}

	client, err := caldav.NewClient(httpClient, d.endpoint)
	if err != nil {
		return fmt.Errorf("caldav-calendar-events: create client: %w", err)
	}
	d.client = client

	return d.calendarEventsPollingDatasource.Start(ctx)
}

func (d *CalDAVCalendarEventsDatasource) fetchEvents(ctx context.Context) ([]CalendarEvent, error) {
	if d.client == nil {
		return nil, fmt.Errorf("caldav-calendar-events: client is not initialized")
	}
	if d.calendarPath == "" {
		path, err := d.resolveCalendarPath(ctx)
		if err != nil {
			return nil, err
		}
		d.calendarPath = path
	}
	d.source = d.calendarPath

	start, end := d.queryWindow()
	// go-webdav formats these values with a literal Z; convert before encoding.
	start, end = start.UTC(), end.UTC()
	query := &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{
			Name:     "VCALENDAR",
			AllProps: true,
			AllComps: false,
			Expand:   &caldav.CalendarExpandRequest{Start: start, End: end},
			Comps: []caldav.CalendarCompRequest{
				{
					Name:     "VEVENT",
					AllProps: true,
					AllComps: true,
				},
			},
		},
		CompFilter: caldav.CompFilter{
			Name: "VCALENDAR",
			Comps: []caldav.CompFilter{
				{
					Name:  "VEVENT",
					Start: start,
					End:   end,
				},
			},
		},
	}

	objects, err := d.client.QueryCalendar(ctx, d.calendarPath, query)
	if err != nil {
		return nil, fmt.Errorf("caldav-calendar-events: query %q: %w", d.calendarPath, err)
	}

	events := make([]CalendarEvent, 0, len(objects))
	for _, object := range objects {
		if object.Data == nil {
			continue
		}
		events = append(events, convertICalCalendar(object.Data, d.location, start, end)...)
	}
	return events, nil
}

func (d *CalDAVCalendarEventsDatasource) resolveCalendarPath(ctx context.Context) (string, error) {
	principal, err := d.client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return "", fmt.Errorf("caldav-calendar-events: find current user principal: %w", err)
	}
	homeSet, err := d.client.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return "", fmt.Errorf("caldav-calendar-events: find calendar home set: %w", err)
	}
	calendars, err := d.client.FindCalendars(ctx, homeSet)
	if err != nil {
		return "", fmt.Errorf("caldav-calendar-events: list calendars: %w", err)
	}

	for _, calendar := range calendars {
		if d.calendarName != "" && calendar.Name == d.calendarName && supportsEventComponent(calendar.SupportedComponentSet) {
			return calendar.Path, nil
		}
	}
	if d.calendarName != "" {
		return "", fmt.Errorf("caldav-calendar-events: calendar %q not found or does not support events", d.calendarName)
	}
	for _, calendar := range calendars {
		if supportsEventComponent(calendar.SupportedComponentSet) {
			return calendar.Path, nil
		}
	}
	return "", fmt.Errorf("caldav-calendar-events: no matching calendar found")
}

func supportsEventComponent(components []string) bool {
	if len(components) == 0 {
		return true
	}
	for _, component := range components {
		if strings.EqualFold(component, "VEVENT") {
			return true
		}
	}
	return false
}
