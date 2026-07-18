package udb_plugin_temporal

import (
	udb_plugin_library "github.com/benwiebe/udb-plugin-library"
	"github.com/benwiebe/udb-plugin-library/types"
	"github.com/benwiebe/udb-plugin-temporal/boards"
	"github.com/benwiebe/udb-plugin-temporal/datasources"
)

type UdbPluginTemporal struct{}

func init() {
	udb_plugin_library.Register(&UdbPluginTemporal{})
}

func (u UdbPluginTemporal) GetId() string {
	return "udb-plugin-temporal"
}

func (u UdbPluginTemporal) GetName() string {
	return "UDB Temporal"
}

func (u UdbPluginTemporal) Configure(config types.PluginConfig) error {
	//TODO implement me
	return nil
}

func (u UdbPluginTemporal) GetBoardMap() map[string]types.BoardFactory {
	return map[string]types.BoardFactory{
		"digital-clock": func() types.Board { return boards.NewDigitalClockBoard() },
		"date":          func() types.Board { return boards.NewDateBoard() },
		"clock-date":    func() types.Board { return boards.NewClockDateBoard() },
		"countdown":     func() types.Board { return boards.NewCountdownBoard() },
	}
}

func (u UdbPluginTemporal) GetDatasourceMap() map[string]types.DatasourceFactory {
	return map[string]types.DatasourceFactory{
		"current-time": func() types.Datasource { return &datasources.CurrentTimeDatasource{} },
	}
}
