// Package udb_plugin_temporal registers time, date, countdown, and calendar boards
// and datasources with the Universal Display Board (UDB) system.
//
// It provides the entry point for plugin registration via udb_plugin_library.Register.
package udb_plugin_temporal

import (
	udb_plugin_library "github.com/benwiebe/udb-plugin-library"
	"github.com/benwiebe/udb-plugin-library/types"
	"github.com/benwiebe/udb-plugin-temporal/boards"
	"github.com/benwiebe/udb-plugin-temporal/datasources"
)

// UdbPluginTemporal implements the types.Plugin interface, registering all
// temporal datasources (current-time, ical-calendar-events, caldav-calendar-events)
// and boards (digital-clock, date, clock-date, countdown, upcoming-events).
type UdbPluginTemporal struct{}

func init() {
	udb_plugin_library.Register(&UdbPluginTemporal{})
}

// GetId returns the unique identifier for the temporal plugin ("udb-plugin-temporal").
func (u UdbPluginTemporal) GetId() string {
	return "udb-plugin-temporal"
}

// GetName returns the human-readable display name of the plugin ("UDB Temporal").
func (u UdbPluginTemporal) GetName() string {
	return "UDB Temporal"
}

// Configure applies top-level plugin configuration. Temporal requires no global configuration.
func (u UdbPluginTemporal) Configure(config types.PluginConfig) error {
	return nil
}

// GetBoardMap returns factory functions for all boards provided by this plugin.
func (u UdbPluginTemporal) GetBoardMap() map[string]types.BoardFactory {
	return map[string]types.BoardFactory{
		"digital-clock":   func() types.Board { return boards.NewDigitalClockBoard() },
		"date":            func() types.Board { return boards.NewDateBoard() },
		"clock-date":      func() types.Board { return boards.NewClockDateBoard() },
		"countdown":       func() types.Board { return boards.NewCountdownBoard() },
		"upcoming-events": func() types.Board { return boards.NewUpcomingEventsBoard() },
	}
}

// GetDatasourceMap returns factory functions for all datasources provided by this plugin.
func (u UdbPluginTemporal) GetDatasourceMap() map[string]types.DatasourceFactory {
	return map[string]types.DatasourceFactory{
		"current-time":           func() types.Datasource { return &datasources.CurrentTimeDatasource{} },
		"ical-calendar-events":   func() types.Datasource { return datasources.NewICalCalendarEventsDatasource() },
		"caldav-calendar-events": func() types.Datasource { return datasources.NewCalDAVCalendarEventsDatasource() },
	}
}
