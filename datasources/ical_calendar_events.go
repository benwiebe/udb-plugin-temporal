package datasources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
)

type ICalCalendarEventsDatasource struct {
	calendarEventsPollingDatasource
	client              *http.Client
	etag                string
	lastModified        string
	maxBytes            int64
	sourceURL           string
	sourceModTime       time.Time
	sourceSize          int64
	calendar            *ical.Calendar
	pendingETag         string
	pendingLastModified string
	pendingModTime      time.Time
	pendingSize         int64
}

func NewICalCalendarEventsDatasource() *ICalCalendarEventsDatasource {
	return &ICalCalendarEventsDatasource{
		calendarEventsPollingDatasource: newCalendarEventsPollingDatasource(),
		client:                          &http.Client{Timeout: 15 * time.Second},
		maxBytes:                        8 << 20,
	}
}

func (d *ICalCalendarEventsDatasource) GetId() string {
	return "benwiebe/udb-plugin-temporal/ical-calendar-events"
}

func (d *ICalCalendarEventsDatasource) GetName() string {
	return "iCalendar Events"
}

func (d *ICalCalendarEventsDatasource) GetType() string {
	return CalendarEventsType
}

func (d *ICalCalendarEventsDatasource) Init(cfg json.RawMessage) error {
	var parsed struct {
		URL      string `json:"url"`
		Path     string `json:"path"`
		MaxBytes *int64 `json:"maxBytes"`
		calendarEventsPollingConfig
	}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return err
		}
	}

	if parsed.URL != "" && parsed.Path != "" {
		return fmt.Errorf("ical-calendar-events: specify only one of url or path")
	}
	switch {
	case parsed.URL != "":
		d.source = parsed.URL
		d.sourceURL = normalizeCalendarURL(parsed.URL)
		u, err := url.Parse(d.sourceURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("ical-calendar-events: url must be an HTTP(S) or webcal feed URL")
		}
	case parsed.Path != "":
		d.source = parsed.Path
	default:
		return fmt.Errorf("ical-calendar-events: url or path is required")
	}
	if parsed.MaxBytes != nil {
		if *parsed.MaxBytes <= 0 {
			return fmt.Errorf("ical-calendar-events: maxBytes must be > 0")
		}
		d.maxBytes = *parsed.MaxBytes
	}

	if err := d.initCommon(parsed.calendarEventsPollingConfig); err != nil {
		return err
	}
	d.fetcher = d.fetchEvents
	return nil
}

func (d *ICalCalendarEventsDatasource) fetchEvents(ctx context.Context) ([]CalendarEvent, error) {
	body, modified, err := d.readSource(ctx)
	if err != nil {
		return nil, err
	}
	if modified {
		cal, err := ical.NewDecoder(bytes.NewReader(body)).Decode()
		if err != nil {
			return nil, fmt.Errorf("ical-calendar-events: decode %q: %w", d.source, err)
		}
		d.calendar = cal
		// Only acknowledge a version after it has been read and parsed successfully.
		d.etag, d.lastModified = d.pendingETag, d.pendingLastModified
		d.sourceModTime, d.sourceSize = d.pendingModTime, d.pendingSize
	}
	if d.calendar == nil {
		return nil, fmt.Errorf("ical-calendar-events: source unchanged without a cached calendar")
	}

	windowStart, windowEnd := d.queryWindow()
	return convertICalCalendar(d.calendar, d.location, windowStart, windowEnd), nil
}

func (d *ICalCalendarEventsDatasource) readSource(ctx context.Context) ([]byte, bool, error) {
	if strings.HasPrefix(d.sourceURL, "http://") || strings.HasPrefix(d.sourceURL, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.sourceURL, nil)
		if err != nil {
			return nil, false, fmt.Errorf("ical-calendar-events: create request: %w", err)
		}
		if d.etag != "" {
			req.Header.Set("If-None-Match", d.etag)
		}
		if d.lastModified != "" {
			req.Header.Set("If-Modified-Since", d.lastModified)
		}
		resp, err := d.client.Do(req)
		if err != nil {
			return nil, false, fmt.Errorf("ical-calendar-events: fetch %q: %w", d.source, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotModified {
			return nil, false, nil
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, false, fmt.Errorf("ical-calendar-events: fetch %q: status %s", d.source, resp.Status)
		}
		if resp.ContentLength > d.maxBytes && d.maxBytes > 0 {
			return nil, false, fmt.Errorf("ical-calendar-events: payload exceeds maxBytes limit of %d", d.maxBytes)
		}
		body, err := readLimited(resp.Body, d.maxBytes)
		if err != nil {
			return nil, false, fmt.Errorf("ical-calendar-events: read %q: %w", d.source, err)
		}
		d.pendingETag = resp.Header.Get("ETag")
		d.pendingLastModified = resp.Header.Get("Last-Modified")
		return body, true, nil
	}

	info, err := os.Stat(d.source)
	if err != nil {
		return nil, false, fmt.Errorf("ical-calendar-events: stat %q: %w", d.source, err)
	}
	if !d.sourceModTime.IsZero() && info.ModTime().Equal(d.sourceModTime) && info.Size() == d.sourceSize {
		return nil, false, nil
	}

	file, err := os.Open(d.source)
	if err != nil {
		return nil, false, fmt.Errorf("ical-calendar-events: open %q: %w", d.source, err)
	}
	defer file.Close()

	body, err := readLimited(file, d.maxBytes)
	if err != nil {
		return nil, false, fmt.Errorf("ical-calendar-events: read %q: %w", d.source, err)
	}

	d.pendingModTime = info.ModTime()
	d.pendingSize = info.Size()
	return body, true, nil
}

func normalizeCalendarURL(raw string) string {
	switch {
	case strings.HasPrefix(raw, "webcals://"):
		return "https://" + strings.TrimPrefix(raw, "webcals://")
	case strings.HasPrefix(raw, "webcal://"):
		return "https://" + strings.TrimPrefix(raw, "webcal://")
	default:
		return raw
	}
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return io.ReadAll(r)
	}

	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("calendar payload exceeds maxBytes limit of %d", maxBytes)
	}
	return body, nil
}
