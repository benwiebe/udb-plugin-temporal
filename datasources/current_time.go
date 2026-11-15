// Package datasources provides temporal datasources for UDB, providing real-time clock
// timestamps and polling/caching integrations for iCalendar (.ics) and CalDAV event feeds.
package datasources

import (
	"context"
	"encoding/json"
	"time"
)

// CurrentTimeType is the datasource type identifier for current time providers.
const CurrentTimeType = "benwiebe/udb-plugin-temporal/current-time"

// CurrentTimeDatasource supplies the current local timestamp on demand for clock, date,
// and countdown boards.
type CurrentTimeDatasource struct{}

// GetId returns the unique datasource identifier ("benwiebe/udb-plugin-temporal/current-time").
func (c *CurrentTimeDatasource) GetId() string {
	return CurrentTimeType
}

// GetName returns the human-readable display name ("Current Time").
func (c *CurrentTimeDatasource) GetName() string {
	return "Current Time"
}

// GetType returns the datasource type string (CurrentTimeType).
func (c *CurrentTimeDatasource) GetType() string {
	return CurrentTimeType
}

// Init initializes the datasource. CurrentTimeDatasource requires no configuration.
func (c *CurrentTimeDatasource) Init(_ json.RawMessage) error {
	return nil
}

// GetData returns the current system time as a time.Time value.
func (c *CurrentTimeDatasource) GetData() any {
	return time.Now()
}

// Start begins any background workers. CurrentTimeDatasource is stateless and requires no background routine.
func (c *CurrentTimeDatasource) Start(_ context.Context) error { return nil }

// DataChanged returns nil because GetData computes fresh timestamps synchronously on every render.
func (c *CurrentTimeDatasource) DataChanged() <-chan struct{} { return nil }
