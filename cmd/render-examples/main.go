// render-examples creates actual board renders for the README, enlarged with
// nearest-neighbor scaling so the individual LED pixels remain visible.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/benwiebe/udb-plugin-library/types"
	"github.com/benwiebe/udb-plugin-temporal/boards"
	"github.com/benwiebe/udb-plugin-temporal/datasources"
	xdraw "golang.org/x/image/draw"
)

type snapshotDatasource struct {
	data any
	kind string
}

func (d snapshotDatasource) GetId() string                { return "snapshot" }
func (d snapshotDatasource) GetName() string              { return "Snapshot" }
func (d snapshotDatasource) GetType() string              { return d.kind }
func (d snapshotDatasource) Init(json.RawMessage) error   { return nil }
func (d snapshotDatasource) GetData() any                 { return d.data }
func (d snapshotDatasource) Start(context.Context) error  { return nil }
func (d snapshotDatasource) DataChanged() <-chan struct{} { return nil }

func main() {
	if err := render(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func render() error {
	dir := "docs/screenshots"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	now := time.Date(2030, 1, 2, 15, 4, 6, 0, time.UTC)
	clock := snapshotDatasource{data: now, kind: datasources.CurrentTimeType}
	calendar := snapshotDatasource{kind: datasources.CalendarEventsType, data: datasources.CalendarEventsData{LastUpdated: now, Events: []datasources.CalendarEvent{
		{ID: "meeting", Title: "Team planning", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour), Location: "Office"},
		{ID: "movie", Title: "Family movie night", Start: now.Add(4 * time.Hour), End: now.Add(6 * time.Hour), Location: "Living room"},
		{ID: "day", Title: "Planning day", Start: now.AddDate(0, 0, 1), End: now.AddDate(0, 0, 2), AllDay: true},
	}}}
	cases := []struct {
		name   string
		board  types.DynamicBoard
		config string
		ds     types.Datasource
		dims   types.BoardDimensions
	}{
		{"digital-clock", boards.NewDigitalClockBoard(), `{"timezone":"UTC","colour":"#64B4FF","padding":2}`, clock, types.BoardDimensions{Width: 64, Height: 32}},
		{"date", boards.NewDateBoard(), `{"timezone":"UTC","padding":2}`, clock, types.BoardDimensions{Width: 64, Height: 32}},
		{"clock-date", boards.NewClockDateBoard(), `{"timezone":"UTC","dateColour":"#64B4FF","padding":2}`, clock, types.BoardDimensions{Width: 64, Height: 32}},
		{"countdown", boards.NewCountdownBoard(), `{"target":"2030-01-06T00:00:00Z","label":"Birthday","labelColour":"#64B4FF","padding":2,"showSeconds":false}`, clock, types.BoardDimensions{Width: 64, Height: 32}},
		{"upcoming-events", boards.NewUpcomingEventsBoard(), `{"timezone":"UTC","padding":2}`, calendar, types.BoardDimensions{Width: 64, Height: 32}},
		{"upcoming-events-list", boards.NewUpcomingEventsBoard(), `{"timezone":"UTC","padding":2,"maxEvents":3,"showLocation":true}`, calendar, types.BoardDimensions{Width: 128, Height: 64}},
	}
	for _, example := range cases {
		if err := example.board.Init(json.RawMessage(example.config), example.ds, example.dims); err != nil {
			return fmt.Errorf("%s: %w", example.name, err)
		}
		img := example.board.Render().Img
		scale := 8
		if example.dims.Width > 64 {
			scale = 4
		}
		scaled := image.NewRGBA(image.Rect(0, 0, example.dims.Width*scale, example.dims.Height*scale))
		xdraw.NearestNeighbor.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Src, nil)
		path := filepath.Join(dir, example.name+".png")
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		err = png.Encode(file, scaled)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println(path)
	}
	return nil
}
