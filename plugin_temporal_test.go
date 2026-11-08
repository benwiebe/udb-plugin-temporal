package udb_plugin_temporal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/benwiebe/udb-plugin-library/types"
)

func TestExampleConfigurationInitializesAndRendersAllBoards(t *testing.T) {
	var config struct {
		Display struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"display"`
		Datasources []struct {
			ID           string          `json:"id"`
			DatasourceID string          `json:"datasourceId"`
			Config       json.RawMessage `json:"config"`
		} `json:"datasources"`
		Boards []struct {
			BoardID    string          `json:"boardId"`
			Datasource string          `json:"datasource"`
			Config     json.RawMessage `json:"config"`
		} `json:"boards"`
	}
	data, err := os.ReadFile(filepath.Join("examples", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	plugin := UdbPluginTemporal{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	datasources := make(map[string]types.Datasource)
	for _, entry := range config.Datasources {
		factory := plugin.GetDatasourceMap()[entry.DatasourceID]
		if factory == nil {
			t.Fatalf("missing datasource %s", entry.DatasourceID)
		}
		ds := factory()
		if err := ds.Init(entry.Config); err != nil {
			t.Fatal(err)
		}
		if err := ds.Start(ctx); err != nil {
			t.Fatal(err)
		}
		datasources[entry.ID] = ds
	}
	for _, entry := range config.Boards {
		factory := plugin.GetBoardMap()[entry.BoardID]
		if factory == nil {
			t.Fatalf("missing board %s", entry.BoardID)
		}
		b := factory()
		if err := b.Init(entry.Config, datasources[entry.Datasource], types.BoardDimensions{Width: config.Display.Width, Height: config.Display.Height}); err != nil {
			t.Fatalf("%s: %v", entry.BoardID, err)
		}
		frame := b.(types.DynamicBoard).Render()
		if frame.Img == nil || frame.Duration <= 0 || frame.Img.Bounds().Dx() != config.Display.Width || frame.Img.Bounds().Dy() != config.Display.Height {
			t.Fatalf("invalid %s frame", entry.BoardID)
		}
	}
}
