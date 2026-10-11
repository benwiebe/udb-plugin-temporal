package datasources

import (
	"context"
	"encoding/json"
	"time"
)

const CurrentTimeType = "benwiebe/udb-plugin-temporal/current-time"

// CurrentTimeDatasource returns the current local time on demand.
type CurrentTimeDatasource struct{}

func (c *CurrentTimeDatasource) GetId() string {
	return CurrentTimeType
}

func (c *CurrentTimeDatasource) GetName() string {
	return "Current Time"
}

func (c *CurrentTimeDatasource) GetType() string {
	return CurrentTimeType
}

func (c *CurrentTimeDatasource) Init(_ json.RawMessage) error {
	return nil
}

func (c *CurrentTimeDatasource) GetData() any {
	return time.Now()
}

func (c *CurrentTimeDatasource) Start(_ context.Context) error { return nil }

// DataChanged returns nil because GetData computes a fresh time for every render.
func (c *CurrentTimeDatasource) DataChanged() <-chan struct{} { return nil }
