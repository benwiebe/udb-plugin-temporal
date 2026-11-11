package boards

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"time"

	"github.com/benwiebe/udb-plugin-library/types"
	"github.com/benwiebe/udb-plugin-temporal/datasources"
	"github.com/benwiebe/udb-plugin-temporal/helpers"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// DateBoard displays the current date from a CurrentTimeDatasource.
// Optional JSON config fields:
//   - format:            Go time layout string (default "Mon Jan 2")
//   - colour:            hex colour for the text, e.g. "#FFFFFF" (default white)
//   - backgroundColour:  hex colour for the background, e.g. "#000000" (default black)
//   - padding:           padding in pixels on all sides (default 10% of width/height)
//   - fontStyle:         "auto", "dotMatrix", "scoreboard", "condensed", "outlined", "silkscreenBold", or "pixelifySansBold"
//   - fontScale:         integer pixel scale (default 0, automatic)
//   - fillScreen:        use zero padding (default false)
//   - horizontalAlign:   "left", "center", or "right" (default "center")
//   - verticalAlign:     "top", "center", or "bottom" (default "center")
//   - timezone:          IANA timezone name, e.g. "America/Toronto" (default local time)
//   - uppercase:         render the formatted date in uppercase (default false)
type DateBoard struct {
	datasource       types.Datasource
	format           string
	colour           color.Color
	backgroundColour color.Color
	horizontalAlign  string
	verticalAlign    string
	location         *time.Location
	padding          *int
	uppercase        bool
	fontOptions      fontOptions

	cachedDims    types.BoardDimensions
	cachedFace    font.Face
	cachedSrc     *image.Uniform
	cachedBg      *image.Uniform
	cachedInner   image.Rectangle
	cachedAscent  int
	cachedHeight  int
	cachedDescent int
}

func NewDateBoard() *DateBoard {
	return &DateBoard{}
}

func (b *DateBoard) GetId() string   { return "date" }
func (b *DateBoard) GetName() string { return "Date" }
func (b *DateBoard) GetSupportedDimensions() []types.BoardDimensions {
	return []types.BoardDimensions{}
}
func (b *DateBoard) GetType() types.BoardType  { return types.BoardTypeDynamic }
func (b *DateBoard) GetDatasourceType() string { return datasources.CurrentTimeType }

func (b *DateBoard) Init(cfg json.RawMessage, datasource types.Datasource, dimensions types.BoardDimensions) error {
	if datasource == nil || datasource.GetType() != datasources.CurrentTimeType {
		return fmt.Errorf("date: expected a current-time datasource, got %T", datasource)
	}

	b.datasource = datasource
	b.format = "Mon Jan 2"
	b.colour = color.White
	b.backgroundColour = color.Black
	b.horizontalAlign = "center"
	b.verticalAlign = "center"

	if len(cfg) > 0 {
		var parsed struct {
			Format           string `json:"format"`
			Colour           string `json:"colour"`
			BackgroundColour string `json:"backgroundColour"`
			Padding          *int   `json:"padding"`
			FontStyle        string `json:"fontStyle"`
			FontScale        *int   `json:"fontScale"`
			FillScreen       *bool  `json:"fillScreen"`
			HorizontalAlign  string `json:"horizontalAlign"`
			VerticalAlign    string `json:"verticalAlign"`
			Timezone         string `json:"timezone"`
			Uppercase        *bool  `json:"uppercase"`
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
		if parsed.Padding != nil {
			if *parsed.Padding < 0 {
				return fmt.Errorf("date: padding must be >= 0")
			}
			b.padding = parsed.Padding
		}
		fontOpts, err := parseFontOptions(parsed.FontStyle, parsed.FontScale, "date")
		if err != nil {
			return err
		}
		b.fontOptions = fontOpts
		if parsed.FillScreen != nil {
			setFillScreen(&b.padding, *parsed.FillScreen)
		}
		if parsed.HorizontalAlign != "" {
			if !isHorizontalAlign(parsed.HorizontalAlign) {
				return fmt.Errorf("date: invalid horizontalAlign %q", parsed.HorizontalAlign)
			}
			b.horizontalAlign = parsed.HorizontalAlign
		}
		if parsed.VerticalAlign != "" {
			if !isVerticalAlign(parsed.VerticalAlign) {
				return fmt.Errorf("date: invalid verticalAlign %q", parsed.VerticalAlign)
			}
			b.verticalAlign = parsed.VerticalAlign
		}
		if parsed.Timezone != "" {
			location, err := time.LoadLocation(parsed.Timezone)
			if err != nil {
				return fmt.Errorf("date: load timezone %q: %w", parsed.Timezone, err)
			}
			b.location = location
		}
		if parsed.Uppercase != nil {
			b.uppercase = *parsed.Uppercase
		}
	}

	if _, ok := datasource.GetData().(time.Time); !ok {
		return fmt.Errorf("datasource must return time.Time")
	}
	if err := validateBoardDimensions(dimensions, b.padding); err != nil {
		return err
	}
	return b.buildCache(dimensions)
}

func (b *DateBoard) Render() types.AnimationFrame {
	t := b.datasource.GetData().(time.Time)
	if b.location != nil {
		t = t.In(b.location)
	}

	dateStr := t.Format(b.format)
	if b.uppercase {
		dateStr = strings.ToUpper(dateStr)
	}

	img := image.NewRGBA(image.Rect(0, 0, b.cachedDims.Width, b.cachedDims.Height))
	draw.Draw(img, img.Bounds(), b.cachedBg, image.Point{}, draw.Src)

	dateStr = truncateText(b.cachedFace, dateStr, b.cachedInner.Dx())
	textWidth := font.MeasureString(b.cachedFace, dateStr).Ceil()
	x := alignedX(b.cachedInner, textWidth, b.horizontalAlign)
	y := alignedBaselineY(b.cachedInner, b.cachedAscent, b.cachedHeight, b.cachedDescent, b.verticalAlign)

	d := font.Drawer{
		Dst:  img.SubImage(b.cachedInner).(draw.Image),
		Src:  b.cachedSrc,
		Face: b.cachedFace,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(dateStr)

	nextMidnight := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
	return types.AnimationFrame{Img: img, Duration: min(time.Minute, nextMidnight.Sub(t))}
}

func (b *DateBoard) buildCache(dimensions types.BoardDimensions) error {
	if b.cachedFace != nil {
		b.cachedFace.Close()
	}
	b.cachedDims = dimensions

	inner := resolveInnerRect(dimensions, b.padding)
	ref := widestTimeSample(b.format, b.uppercase, b.location)
	face, err := configuredTextFace(ref, inner.Dx(), inner.Dy(), b.fontOptions)
	if err != nil {
		return fmt.Errorf("date: %w", err)
	}
	b.cachedFace = face

	m := b.cachedFace.Metrics()
	b.cachedInner = inner
	b.cachedAscent = m.Ascent.Ceil()
	b.cachedDescent = m.Descent.Ceil()
	b.cachedHeight = (m.Ascent + m.Descent).Ceil()
	b.cachedSrc = image.NewUniform(b.colour)
	b.cachedBg = image.NewUniform(b.backgroundColour)
	return nil
}
