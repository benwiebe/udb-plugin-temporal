package boards

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"time"

	"github.com/benwiebe/udb-plugin-library/fonts"
	"github.com/benwiebe/udb-plugin-library/types"
	"github.com/benwiebe/udb-plugin-temporal/datasources"
	"github.com/benwiebe/udb-plugin-temporal/helpers"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// DigitalClockBoard displays the current time from a CurrentTimeDatasource.
// Optional JSON config fields:
//   - format:            Go time layout string (default "15:04"; use "15:04:05" to show seconds)
//   - colour:            hex colour for the digits, e.g. "#FF0000" (default white)
//   - backgroundColour:  hex colour for the background, e.g. "#000000" (default black)
//   - blinkColon:        toggle colon separators off on odd seconds (default true)
//   - padding:           padding in pixels on all sides (default 10% of width/height)
//   - horizontalAlign:   "left", "center", or "right" (default "center")
//   - verticalAlign:     "top", "center", or "bottom" (default "center")
//   - timezone:          IANA timezone name, e.g. "America/Toronto" (default local time)
type DigitalClockBoard struct {
	datasource       *datasources.CurrentTimeDatasource
	format           string
	colour           color.Color
	backgroundColour color.Color
	blinkColon       bool
	horizontalAlign  string
	verticalAlign    string
	location         *time.Location
	padding          *int

	// Pre-computed in Init from the display dimensions and config.
	cachedDims    types.BoardDimensions
	cachedFace    font.Face
	cachedSrc     *image.Uniform
	cachedBg      *image.Uniform
	cachedInner   image.Rectangle
	cachedAscent  int
	cachedHeight  int
	cachedDescent int
}

func NewDigitalClockBoard() *DigitalClockBoard {
	return &DigitalClockBoard{}
}

func (b *DigitalClockBoard) GetId() string   { return "digital-clock" }
func (b *DigitalClockBoard) GetName() string { return "Digital Clock" }
func (b *DigitalClockBoard) GetSupportedDimensions() []types.BoardDimensions {
	return []types.BoardDimensions{}
}
func (b *DigitalClockBoard) GetType() types.BoardType  { return types.BoardTypeDynamic }
func (b *DigitalClockBoard) GetDatasourceType() string { return datasources.CurrentTimeType }

func (b *DigitalClockBoard) Init(cfg json.RawMessage, datasource types.Datasource, dimensions types.BoardDimensions) error {
	typedDatasource, ok := datasource.(*datasources.CurrentTimeDatasource)
	if !ok {
		return fmt.Errorf("digital-clock: expected *datasources.CurrentTimeDatasource, got %T", datasource)
	}

	b.datasource = typedDatasource
	b.format = "15:04"
	b.colour = color.White
	b.backgroundColour = color.Black
	b.blinkColon = true
	b.horizontalAlign = "center"
	b.verticalAlign = "center"

	if len(cfg) > 0 {
		var parsed struct {
			Format           string `json:"format"`
			Colour           string `json:"colour"`
			BackgroundColour string `json:"backgroundColour"`
			BlinkColon       *bool  `json:"blinkColon"`
			Padding          *int   `json:"padding"`
			HorizontalAlign  string `json:"horizontalAlign"`
			VerticalAlign    string `json:"verticalAlign"`
			Timezone         string `json:"timezone"`
		}
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return err
		}
		if parsed.Format != "" {
			b.format = parsed.Format
		}
		if parsed.Colour != "" {
			c, err := helpers.ParseHexColour(parsed.Colour)
			if err != nil {
				return err
			}
			b.colour = c
		}
		if parsed.BackgroundColour != "" {
			c, err := helpers.ParseHexColour(parsed.BackgroundColour)
			if err != nil {
				return err
			}
			b.backgroundColour = c
		}
		if parsed.BlinkColon != nil {
			b.blinkColon = *parsed.BlinkColon
		}
		if parsed.Padding != nil {
			if *parsed.Padding < 0 {
				return fmt.Errorf("digital-clock: padding must be >= 0")
			}
			b.padding = parsed.Padding
		}
		if parsed.HorizontalAlign != "" {
			if !isHorizontalAlign(parsed.HorizontalAlign) {
				return fmt.Errorf("digital-clock: invalid horizontalAlign %q", parsed.HorizontalAlign)
			}
			b.horizontalAlign = parsed.HorizontalAlign
		}
		if parsed.VerticalAlign != "" {
			if !isVerticalAlign(parsed.VerticalAlign) {
				return fmt.Errorf("digital-clock: invalid verticalAlign %q", parsed.VerticalAlign)
			}
			b.verticalAlign = parsed.VerticalAlign
		}
		if parsed.Timezone != "" {
			location, err := time.LoadLocation(parsed.Timezone)
			if err != nil {
				return fmt.Errorf("digital-clock: load timezone %q: %w", parsed.Timezone, err)
			}
			b.location = location
		}
	}

	b.buildCache(dimensions)
	return nil
}

func (b *DigitalClockBoard) Render() types.AnimationFrame {
	t := b.datasource.GetData().(time.Time)
	if b.location != nil {
		t = t.In(b.location)
	}

	timeStr := t.Format(b.format)
	if b.blinkColon && t.Second()%2 == 1 {
		timeStr = strings.ReplaceAll(timeStr, ":", " ")
	}

	img := image.NewRGBA(image.Rect(0, 0, b.cachedDims.Width, b.cachedDims.Height))
	draw.Draw(img, img.Bounds(), b.cachedBg, image.Point{}, draw.Src)

	textWidth := font.MeasureString(b.cachedFace, timeStr).Ceil()
	x := alignedX(b.cachedInner, textWidth, b.horizontalAlign)
	y := alignedBaselineY(b.cachedInner, b.cachedAscent, b.cachedHeight, b.cachedDescent, b.verticalAlign)

	d := font.Drawer{
		Dst:  img,
		Src:  b.cachedSrc,
		Face: b.cachedFace,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(timeStr)

	// Use 500ms when blinkColon is on or when the format includes seconds — sub-second
	// precision is needed in both cases. Otherwise 1s is sufficient.
	dur := time.Second
	if b.blinkColon || strings.Contains(b.format, "05") {
		dur = 500 * time.Millisecond
	}
	return types.AnimationFrame{Img: img, Duration: dur}
}

// buildCache computes and stores the font face, baseline origin, and colour source for the
// given dimensions. Called once from Init.
func (b *DigitalClockBoard) buildCache(dimensions types.BoardDimensions) {
	if b.cachedFace != nil {
		b.cachedFace.Close()
	}
	b.cachedDims = dimensions

	inner := resolveInnerRect(dimensions, b.padding)

	// Use the Go time reference moment so the sample string matches the exact layout
	// (same character count and structure as any real formatted time).
	ref := time.Date(0, 1, 1, 15, 4, 5, 0, time.UTC).Format(b.format)
	fontSize := findTextFontSize(ref, inner.Dx(), inner.Dy())

	b.cachedFace = truetype.NewFace(
		fonts.GetUdbFonts().FontForSize(int(fontSize)),
		&truetype.Options{Size: fontSize, DPI: 72})

	m := b.cachedFace.Metrics()
	b.cachedInner = inner
	b.cachedAscent = m.Ascent.Ceil()
	b.cachedDescent = m.Descent.Ceil()
	b.cachedHeight = (m.Ascent + m.Descent).Ceil()
	b.cachedSrc = image.NewUniform(b.colour)
	b.cachedBg = image.NewUniform(b.backgroundColour)
}
