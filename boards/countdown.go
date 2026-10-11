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
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// CountdownBoard displays the time remaining until a target date/time.
// Optional JSON config fields:
//   - target:            target timestamp. Supported formats include RFC3339, "2006-01-02 15:04:05",
//     "2006-01-02 15:04", and "2006-01-02". Required.
//   - timezone:          IANA timezone used to interpret zone-less targets and current time.
//   - label:             optional second line shown below the countdown
//   - colour:            fallback hex colour for both lines, e.g. "#FFFFFF" (default white)
//   - labelColour:       hex colour for the label line
//   - backgroundColour:  hex colour for the background, e.g. "#000000" (default black)
//   - padding:           padding in pixels on all sides (default 10% of width/height)
//   - horizontalAlign:   "left", "center", or "right" (default "center")
//   - verticalAlign:     "top", "center", or "bottom" (default "center")
//   - showSeconds:       include seconds in the countdown (default true)
//   - expiredText:       text to render once the target has passed (default "NOW")
//   - labelUppercase:    render the label in uppercase (default false)
//   - labelScale:        relative label font scale versus countdown font (default 0.35)
//   - lineGap:           gap in pixels between countdown and label lines (default 2)
type CountdownBoard struct {
	datasource       *datasources.CurrentTimeDatasource
	target           time.Time
	location         *time.Location
	label            string
	countdownColour  color.Color
	labelColour      color.Color
	backgroundColour color.Color
	horizontalAlign  string
	verticalAlign    string
	padding          *int
	showSeconds      bool
	expiredText      string
	labelUppercase   bool
	labelScale       float64
	lineGap          int

	cachedDims             types.BoardDimensions
	cachedCountdownFace    font.Face
	cachedLabelFace        font.Face
	cachedCountdownSrc     *image.Uniform
	cachedLabelSrc         *image.Uniform
	cachedBg               *image.Uniform
	cachedInner            image.Rectangle
	cachedLabelLines       []string
	cachedCountdownAscent  int
	cachedCountdownDescent int
	cachedCountdownHeight  int
	cachedLabelAscent      int
	cachedLabelHeight      int
	cachedCombinedHeight   int
}

func NewCountdownBoard() *CountdownBoard {
	return &CountdownBoard{}
}

func (b *CountdownBoard) GetId() string   { return "countdown" }
func (b *CountdownBoard) GetName() string { return "Countdown" }
func (b *CountdownBoard) GetSupportedDimensions() []types.BoardDimensions {
	return []types.BoardDimensions{}
}
func (b *CountdownBoard) GetType() types.BoardType  { return types.BoardTypeDynamic }
func (b *CountdownBoard) GetDatasourceType() string { return datasources.CurrentTimeType }

func (b *CountdownBoard) Init(cfg json.RawMessage, datasource types.Datasource, dimensions types.BoardDimensions) error {
	typedDatasource, ok := datasource.(*datasources.CurrentTimeDatasource)
	if !ok {
		return fmt.Errorf("countdown: expected *datasources.CurrentTimeDatasource, got %T", datasource)
	}

	b.datasource = typedDatasource
	b.countdownColour = color.White
	b.labelColour = color.White
	b.backgroundColour = color.Black
	b.horizontalAlign = "center"
	b.verticalAlign = "center"
	b.showSeconds = true
	b.expiredText = "NOW"
	b.labelScale = 0.35
	b.lineGap = 2

	var parsed struct {
		Target           string   `json:"target"`
		Timezone         string   `json:"timezone"`
		Label            string   `json:"label"`
		Colour           string   `json:"colour"`
		LabelColour      string   `json:"labelColour"`
		BackgroundColour string   `json:"backgroundColour"`
		Padding          *int     `json:"padding"`
		HorizontalAlign  string   `json:"horizontalAlign"`
		VerticalAlign    string   `json:"verticalAlign"`
		ShowSeconds      *bool    `json:"showSeconds"`
		ExpiredText      string   `json:"expiredText"`
		LabelUppercase   *bool    `json:"labelUppercase"`
		LabelScale       *float64 `json:"labelScale"`
		LineGap          *int     `json:"lineGap"`
	}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return err
		}
	}

	if parsed.Target == "" {
		return fmt.Errorf("countdown: target is required")
	}
	if parsed.Timezone != "" {
		location, err := time.LoadLocation(parsed.Timezone)
		if err != nil {
			return fmt.Errorf("countdown: load timezone %q: %w", parsed.Timezone, err)
		}
		b.location = location
	}
	target, err := parseCountdownTarget(parsed.Target, b.location)
	if err != nil {
		return err
	}
	b.target = target
	b.label = parsed.Label

	if parsed.Colour != "" {
		c, err := helpers.ParseHexColour(parsed.Colour)
		if err != nil {
			return err
		}
		b.countdownColour = c
		b.labelColour = c
	}
	if parsed.LabelColour != "" {
		c, err := helpers.ParseHexColour(parsed.LabelColour)
		if err != nil {
			return err
		}
		b.labelColour = c
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
			return fmt.Errorf("countdown: padding must be >= 0")
		}
		b.padding = parsed.Padding
	}
	if parsed.HorizontalAlign != "" {
		if !isHorizontalAlign(parsed.HorizontalAlign) {
			return fmt.Errorf("countdown: invalid horizontalAlign %q", parsed.HorizontalAlign)
		}
		b.horizontalAlign = parsed.HorizontalAlign
	}
	if parsed.VerticalAlign != "" {
		if !isVerticalAlign(parsed.VerticalAlign) {
			return fmt.Errorf("countdown: invalid verticalAlign %q", parsed.VerticalAlign)
		}
		b.verticalAlign = parsed.VerticalAlign
	}
	if parsed.ShowSeconds != nil {
		b.showSeconds = *parsed.ShowSeconds
	}
	if parsed.ExpiredText != "" {
		b.expiredText = parsed.ExpiredText
	}
	if parsed.LabelUppercase != nil {
		b.labelUppercase = *parsed.LabelUppercase
	}
	if parsed.LabelScale != nil {
		if *parsed.LabelScale <= 0 {
			return fmt.Errorf("countdown: labelScale must be > 0")
		}
		b.labelScale = *parsed.LabelScale
	}
	if parsed.LineGap != nil {
		if *parsed.LineGap < 0 {
			return fmt.Errorf("countdown: lineGap must be >= 0")
		}
		b.lineGap = *parsed.LineGap
	}

	b.buildCache(dimensions)
	return nil
}

func (b *CountdownBoard) Render() types.AnimationFrame {
	now := b.datasource.GetData().(time.Time)
	if b.location != nil {
		now = now.In(b.location)
	}

	countdownText := formatCountdownText(b.target.Sub(now), b.showSeconds, b.expiredText)
	labelText := b.renderLabel()

	img := image.NewRGBA(image.Rect(0, 0, b.cachedDims.Width, b.cachedDims.Height))
	draw.Draw(img, img.Bounds(), b.cachedBg, image.Point{}, draw.Src)

	if labelText == "" {
		textWidth := font.MeasureString(b.cachedCountdownFace, countdownText).Ceil()
		x := alignedX(b.cachedInner, textWidth, b.horizontalAlign)
		y := alignedBaselineY(b.cachedInner, b.cachedCountdownAscent, b.cachedCountdownHeight, b.cachedCountdownDescent, b.verticalAlign)
		drawTextLine(img, b.cachedCountdownFace, b.cachedCountdownSrc, countdownText, x, y)
	} else {
		blockTop := alignedBlockTop(b.cachedInner, b.cachedCombinedHeight, b.verticalAlign)

		countdownWidth := font.MeasureString(b.cachedCountdownFace, countdownText).Ceil()
		countdownX := alignedX(b.cachedInner, countdownWidth, b.horizontalAlign)
		countdownY := blockTop + b.cachedCountdownAscent
		drawTextLine(img, b.cachedCountdownFace, b.cachedCountdownSrc, countdownText, countdownX, countdownY)

		labelY := blockTop + b.cachedCountdownHeight + b.lineGap + b.cachedLabelAscent
		for _, line := range b.cachedLabelLines {
			labelWidth := font.MeasureString(b.cachedLabelFace, line).Ceil()
			labelX := alignedX(b.cachedInner, labelWidth, b.horizontalAlign)
			drawTextLine(img, b.cachedLabelFace, b.cachedLabelSrc, line, labelX, labelY)
			labelY += b.cachedLabelHeight + b.lineGap
		}
	}

	if b.showSeconds {
		return types.AnimationFrame{Img: img, Duration: time.Second}
	}
	return types.AnimationFrame{Img: img, Duration: time.Minute}
}

func (b *CountdownBoard) buildCache(dimensions types.BoardDimensions) {
	if b.cachedCountdownFace != nil {
		b.cachedCountdownFace.Close()
	}
	if b.cachedLabelFace != nil {
		b.cachedLabelFace.Close()
	}
	b.cachedLabelLines = nil

	b.cachedDims = dimensions
	b.cachedInner = resolveInnerRect(dimensions, b.padding)
	b.cachedBg = image.NewUniform(b.backgroundColour)
	b.cachedCountdownSrc = image.NewUniform(b.countdownColour)
	b.cachedLabelSrc = image.NewUniform(b.labelColour)

	countdownSample := maxCountdownSample(formatCountdownText(b.target.Sub(time.Now()), b.showSeconds, b.expiredText), b.showSeconds, b.expiredText)
	labelText := b.renderLabel()
	if labelText == "" {
		b.cachedCountdownFace = findTextFace(countdownSample, b.cachedInner.Dx(), b.cachedInner.Dy())
		metrics := b.cachedCountdownFace.Metrics()
		b.cachedCountdownAscent = metrics.Ascent.Ceil()
		b.cachedCountdownDescent = metrics.Descent.Ceil()
		b.cachedCountdownHeight = (metrics.Ascent + metrics.Descent).Ceil()
		b.cachedCombinedHeight = b.cachedCountdownHeight
		return
	}

	b.cachedCountdownFace = findTextFace(countdownSample, b.cachedInner.Dx(), b.cachedInner.Dy())
	b.cachedLabelFace = fonts.GetUdbFonts().FitPixelFace(labelText, b.cachedInner.Dx(), b.cachedInner.Dy(), fonts.PixelFontAuto)

	countdownMetrics := b.cachedCountdownFace.Metrics()
	labelMetrics := b.cachedLabelFace.Metrics()
	b.cachedLabelLines = wrapTextLines(b.cachedLabelFace, labelText, b.cachedInner.Dx())
	b.cachedCountdownAscent = countdownMetrics.Ascent.Ceil()
	b.cachedCountdownDescent = countdownMetrics.Descent.Ceil()
	b.cachedCountdownHeight = (countdownMetrics.Ascent + countdownMetrics.Descent).Ceil()
	b.cachedLabelAscent = labelMetrics.Ascent.Ceil()
	b.cachedLabelHeight = (labelMetrics.Ascent + labelMetrics.Descent).Ceil()

	labelBlockHeight := len(b.cachedLabelLines) * b.cachedLabelHeight
	if len(b.cachedLabelLines) > 1 {
		labelBlockHeight += (len(b.cachedLabelLines) - 1) * b.lineGap
	}
	b.cachedCombinedHeight = b.cachedCountdownHeight + b.lineGap + labelBlockHeight

	if b.cachedCombinedHeight > b.cachedInner.Dy() {
		b.cachedCountdownFace.Close()
		b.cachedCountdownFace = fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{
			Style:         fonts.PixelFontCondensed,
			Scale:         1,
			LetterSpacing: -1,
		})
		countdownMetrics = b.cachedCountdownFace.Metrics()
		b.cachedCountdownAscent = countdownMetrics.Ascent.Ceil()
		b.cachedCountdownDescent = countdownMetrics.Descent.Ceil()
		b.cachedCountdownHeight = (countdownMetrics.Ascent + countdownMetrics.Descent).Ceil()
		b.cachedCombinedHeight = b.cachedCountdownHeight + b.lineGap + labelBlockHeight
	}
}

func (b *CountdownBoard) renderLabel() string {
	if b.label == "" {
		return ""
	}
	if b.labelUppercase {
		return strings.ToUpper(b.label)
	}
	return b.label
}

func parseCountdownTarget(raw string, location *time.Location) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if strings.Contains(layout, "Z07:00") {
			if t, err := time.Parse(layout, raw); err == nil {
				return t, nil
			}
			continue
		}

		loc := location
		if loc == nil {
			loc = time.Local
		}
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("countdown: unsupported target format %q", raw)
}

func formatCountdownText(remaining time.Duration, showSeconds bool, expiredText string) string {
	if remaining <= 0 {
		return expiredText
	}

	totalSeconds := int64(remaining / time.Second)
	days := totalSeconds / 86400
	hours := (totalSeconds % 86400) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if days > 0 {
		if showSeconds {
			return fmt.Sprintf("%dd %02d:%02d:%02d", days, hours, minutes, seconds)
		}
		return fmt.Sprintf("%dd %02d:%02d", days, hours, minutes)
	}

	totalHours := totalSeconds / 3600
	if showSeconds {
		return fmt.Sprintf("%02d:%02d:%02d", totalHours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", totalHours, minutes)
}

func maxCountdownSample(currentText string, showSeconds bool, expiredText string) string {
	sample := "999d 23:59"
	if showSeconds {
		sample = "999d 23:59:59"
	}
	if len(currentText) > len(sample) {
		sample = currentText
	}
	if len(expiredText) > len(sample) {
		return expiredText
	}
	return sample
}

func drawTextLine(dst draw.Image, face font.Face, src image.Image, text string, x, y int) {
	d := font.Drawer{
		Dst:  dst,
		Src:  src,
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(text)
}
