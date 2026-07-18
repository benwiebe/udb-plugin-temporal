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

// ClockDateBoard displays the current time and date as a stacked two-line layout.
// Optional JSON config fields:
//   - timeFormat:        Go time layout string for the top line (default "15:04")
//   - dateFormat:        Go time layout string for the bottom line (default "Mon Jan 2")
//   - colour:            fallback hex colour for both lines, e.g. "#FFFFFF" (default white)
//   - timeColour:        hex colour for the time line
//   - dateColour:        hex colour for the date line
//   - backgroundColour:  hex colour for the background, e.g. "#000000" (default black)
//   - blinkColon:        toggle colon separators off on odd seconds in the time line (default true)
//   - padding:           padding in pixels on all sides (default 10% of width/height)
//   - horizontalAlign:   "left", "center", or "right" (default "center")
//   - verticalAlign:     "top", "center", or "bottom" (default "center")
//   - timezone:          IANA timezone name, e.g. "America/Toronto" (default local time)
//   - dateUppercase:     render the formatted date in uppercase (default false)
//   - dateScale:         relative date font scale versus time font (default 0.45)
//   - lineGap:           gap in pixels between time and date lines (default 2)
type ClockDateBoard struct {
	datasource       *datasources.CurrentTimeDatasource
	timeFormat       string
	dateFormat       string
	timeColour       color.Color
	dateColour       color.Color
	backgroundColour color.Color
	blinkColon       bool
	horizontalAlign  string
	verticalAlign    string
	location         *time.Location
	padding          *int
	dateUppercase    bool
	dateScale        float64
	lineGap          int

	cachedDims           types.BoardDimensions
	cachedTimeFace       font.Face
	cachedDateFace       font.Face
	cachedTimeSrc        *image.Uniform
	cachedDateSrc        *image.Uniform
	cachedBg             *image.Uniform
	cachedInner          image.Rectangle
	cachedTimeAscent     int
	cachedTimeHeight     int
	cachedTimeDescent    int
	cachedDateAscent     int
	cachedDateHeight     int
	cachedDateDescent    int
	cachedCombinedHeight int
}

func NewClockDateBoard() *ClockDateBoard {
	return &ClockDateBoard{}
}

func (b *ClockDateBoard) GetId() string   { return "clock-date" }
func (b *ClockDateBoard) GetName() string { return "Clock + Date" }
func (b *ClockDateBoard) GetSupportedDimensions() []types.BoardDimensions {
	return []types.BoardDimensions{}
}
func (b *ClockDateBoard) GetType() types.BoardType  { return types.BoardTypeDynamic }
func (b *ClockDateBoard) GetDatasourceType() string { return datasources.CurrentTimeType }

func (b *ClockDateBoard) Init(cfg json.RawMessage, datasource types.Datasource, dimensions types.BoardDimensions) error {
	typedDatasource, ok := datasource.(*datasources.CurrentTimeDatasource)
	if !ok {
		return fmt.Errorf("clock-date: expected *datasources.CurrentTimeDatasource, got %T", datasource)
	}

	b.datasource = typedDatasource
	b.timeFormat = "15:04"
	b.dateFormat = "Mon Jan 2"
	b.timeColour = color.White
	b.dateColour = color.White
	b.backgroundColour = color.Black
	b.blinkColon = true
	b.horizontalAlign = "center"
	b.verticalAlign = "center"
	b.dateScale = 0.45
	b.lineGap = 2

	if len(cfg) > 0 {
		var parsed struct {
			TimeFormat       string   `json:"timeFormat"`
			DateFormat       string   `json:"dateFormat"`
			Colour           string   `json:"colour"`
			TimeColour       string   `json:"timeColour"`
			DateColour       string   `json:"dateColour"`
			BackgroundColour string   `json:"backgroundColour"`
			BlinkColon       *bool    `json:"blinkColon"`
			Padding          *int     `json:"padding"`
			HorizontalAlign  string   `json:"horizontalAlign"`
			VerticalAlign    string   `json:"verticalAlign"`
			Timezone         string   `json:"timezone"`
			DateUppercase    *bool    `json:"dateUppercase"`
			DateScale        *float64 `json:"dateScale"`
			LineGap          *int     `json:"lineGap"`
		}
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return err
		}

		if parsed.TimeFormat != "" {
			b.timeFormat = parsed.TimeFormat
		}
		if parsed.DateFormat != "" {
			b.dateFormat = parsed.DateFormat
		}
		if parsed.Colour != "" {
			c, err := helpers.ParseHexColour(parsed.Colour)
			if err != nil {
				return err
			}
			b.timeColour = c
			b.dateColour = c
		}
		if parsed.TimeColour != "" {
			c, err := helpers.ParseHexColour(parsed.TimeColour)
			if err != nil {
				return err
			}
			b.timeColour = c
		}
		if parsed.DateColour != "" {
			c, err := helpers.ParseHexColour(parsed.DateColour)
			if err != nil {
				return err
			}
			b.dateColour = c
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
				return fmt.Errorf("clock-date: padding must be >= 0")
			}
			b.padding = parsed.Padding
		}
		if parsed.HorizontalAlign != "" {
			if !isHorizontalAlign(parsed.HorizontalAlign) {
				return fmt.Errorf("clock-date: invalid horizontalAlign %q", parsed.HorizontalAlign)
			}
			b.horizontalAlign = parsed.HorizontalAlign
		}
		if parsed.VerticalAlign != "" {
			if !isVerticalAlign(parsed.VerticalAlign) {
				return fmt.Errorf("clock-date: invalid verticalAlign %q", parsed.VerticalAlign)
			}
			b.verticalAlign = parsed.VerticalAlign
		}
		if parsed.Timezone != "" {
			location, err := time.LoadLocation(parsed.Timezone)
			if err != nil {
				return fmt.Errorf("clock-date: load timezone %q: %w", parsed.Timezone, err)
			}
			b.location = location
		}
		if parsed.DateUppercase != nil {
			b.dateUppercase = *parsed.DateUppercase
		}
		if parsed.DateScale != nil {
			if *parsed.DateScale <= 0 {
				return fmt.Errorf("clock-date: dateScale must be > 0")
			}
			b.dateScale = *parsed.DateScale
		}
		if parsed.LineGap != nil {
			if *parsed.LineGap < 0 {
				return fmt.Errorf("clock-date: lineGap must be >= 0")
			}
			b.lineGap = *parsed.LineGap
		}
	}

	b.buildCache(dimensions)
	return nil
}

func (b *ClockDateBoard) Render() types.AnimationFrame {
	t := b.datasource.GetData().(time.Time)
	if b.location != nil {
		t = t.In(b.location)
	}

	timeStr := t.Format(b.timeFormat)
	if b.blinkColon && t.Second()%2 == 1 {
		timeStr = strings.ReplaceAll(timeStr, ":", " ")
	}

	dateStr := t.Format(b.dateFormat)
	if b.dateUppercase {
		dateStr = strings.ToUpper(dateStr)
	}

	img := image.NewRGBA(image.Rect(0, 0, b.cachedDims.Width, b.cachedDims.Height))
	draw.Draw(img, img.Bounds(), b.cachedBg, image.Point{}, draw.Src)

	blockTop := alignedBlockTop(b.cachedInner, b.cachedCombinedHeight, b.verticalAlign)

	timeWidth := font.MeasureString(b.cachedTimeFace, timeStr).Ceil()
	timeX := alignedX(b.cachedInner, timeWidth, b.horizontalAlign)
	timeY := blockTop + b.cachedTimeAscent

	timeDrawer := font.Drawer{
		Dst:  img,
		Src:  b.cachedTimeSrc,
		Face: b.cachedTimeFace,
		Dot:  fixed.P(timeX, timeY),
	}
	timeDrawer.DrawString(timeStr)

	dateWidth := font.MeasureString(b.cachedDateFace, dateStr).Ceil()
	dateX := alignedX(b.cachedInner, dateWidth, b.horizontalAlign)
	dateY := blockTop + b.cachedTimeHeight + b.lineGap + b.cachedDateAscent

	dateDrawer := font.Drawer{
		Dst:  img,
		Src:  b.cachedDateSrc,
		Face: b.cachedDateFace,
		Dot:  fixed.P(dateX, dateY),
	}
	dateDrawer.DrawString(dateStr)

	dur := time.Second
	if b.blinkColon || strings.Contains(b.timeFormat, "05") {
		dur = 500 * time.Millisecond
	}
	return types.AnimationFrame{Img: img, Duration: dur}
}

func (b *ClockDateBoard) buildCache(dimensions types.BoardDimensions) {
	if b.cachedTimeFace != nil {
		b.cachedTimeFace.Close()
	}
	if b.cachedDateFace != nil {
		b.cachedDateFace.Close()
	}
	b.cachedDims = dimensions

	inner := resolveInnerRect(dimensions, b.padding)
	timeRef := time.Date(0, 1, 1, 15, 4, 5, 0, time.UTC).Format(b.timeFormat)
	dateRef := time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC).Format(b.dateFormat)
	if b.dateUppercase {
		dateRef = strings.ToUpper(dateRef)
	}
	fontSize := findStackedTextFontSize(timeRef, dateRef, inner.Dx(), inner.Dy(), b.dateScale, b.lineGap)

	b.cachedTimeFace = truetype.NewFace(
		fonts.GetUdbFonts().FontForSize(int(fontSize)),
		&truetype.Options{Size: fontSize, DPI: 72})
	b.cachedDateFace = truetype.NewFace(
		fonts.GetUdbFonts().FontForSize(int(fontSize*b.dateScale)),
		&truetype.Options{Size: fontSize * b.dateScale, DPI: 72})

	timeMetrics := b.cachedTimeFace.Metrics()
	dateMetrics := b.cachedDateFace.Metrics()

	b.cachedInner = inner
	b.cachedTimeAscent = timeMetrics.Ascent.Ceil()
	b.cachedTimeDescent = timeMetrics.Descent.Ceil()
	b.cachedTimeHeight = (timeMetrics.Ascent + timeMetrics.Descent).Ceil()
	b.cachedDateAscent = dateMetrics.Ascent.Ceil()
	b.cachedDateDescent = dateMetrics.Descent.Ceil()
	b.cachedDateHeight = (dateMetrics.Ascent + dateMetrics.Descent).Ceil()
	b.cachedCombinedHeight = b.cachedTimeHeight + b.lineGap + b.cachedDateHeight
	b.cachedTimeSrc = image.NewUniform(b.timeColour)
	b.cachedDateSrc = image.NewUniform(b.dateColour)
	b.cachedBg = image.NewUniform(b.backgroundColour)
}
