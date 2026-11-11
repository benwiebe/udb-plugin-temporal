package boards

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/benwiebe/udb-plugin-library/fonts"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const outlinedFontStyle fonts.PixelFontStyle = 100

type fontOptions struct {
	style    fonts.PixelFontStyle
	styleSet bool
	scale    int
	scaleSet bool
}

func parseFontOptions(style string, scale *int, board string) (fontOptions, error) {
	options := fontOptions{}
	if style != "" {
		options.styleSet = true
		switch strings.ToLower(style) {
		case "auto":
			options.style = fonts.PixelFontAuto
		case "dotmatrix", "dot-matrix":
			options.style = fonts.PixelFontDotMatrix
		case "scoreboard":
			options.style = fonts.PixelFontScoreboard
		case "condensed":
			options.style = fonts.PixelFontCondensed
		case "outlined", "outline":
			options.style = outlinedFontStyle
		case "silkscreenbold", "silkscreen-bold":
			options.style = fonts.PixelFontSilkscreenBold
		case "pixelifysansbold", "pixelify-sans-bold":
			options.style = fonts.PixelFontPixelifySansBold
		default:
			return fontOptions{}, fmt.Errorf("%s: invalid fontStyle %q", board, style)
		}
	}
	if scale != nil {
		if *scale < 0 {
			return fontOptions{}, fmt.Errorf("%s: fontScale must be >= 0", board)
		}
		options.scale = *scale
		options.scaleSet = true
	}
	return options, nil
}

func setFillScreen(padding **int, fillScreen bool) {
	if fillScreen {
		zero := 0
		*padding = &zero
	}
}

func hasFontOptions(options fontOptions) bool {
	return options.styleSet || options.scaleSet
}

func configuredTextFace(text string, maxWidth, maxHeight int, options fontOptions) (font.Face, error) {
	if !hasFontOptions(options) {
		return findTextFace(text, maxWidth, maxHeight), nil
	}
	if options.scale > maxHeight {
		return nil, fmt.Errorf("fontScale %d is too large for the available height", options.scale)
	}

	style := options.style
	if !options.styleSet || style == fonts.PixelFontAuto {
		style = fonts.PixelFontStyleForTextBounds(text, maxWidth, maxHeight)
	}
	if style == fonts.PixelFontSilkscreenBold || style == fonts.PixelFontPixelifySansBold {
		if options.scale == 0 {
			face := fonts.GetUdbFonts().FitPixelFace(text, maxWidth, maxHeight, style)
			if face.Metrics().Height.Ceil() > maxHeight {
				face.Close()
				return nil, fmt.Errorf("bold pixel font does not fit the available height")
			}
			return face, nil
		}
		face := faceAtScale(style, options.scale)
		if face.Metrics().Height.Ceil() > maxHeight {
			face.Close()
			return nil, fmt.Errorf("fontScale %d is too large for the available height", options.scale)
		}
		return face, nil
	}
	if style == outlinedFontStyle {
		if options.scale == 0 {
			face := fitOutlinedFace(text, maxWidth, maxHeight)
			if face.Metrics().Height.Ceil() > maxHeight {
				face.Close()
				return nil, fmt.Errorf("outlined font does not fit the available height")
			}
			return face, nil
		}
		face := newOutlinedFace(options.scale)
		if face.Metrics().Height.Ceil() > maxHeight {
			face.Close()
			return nil, fmt.Errorf("fontScale %d is too large for the available height", options.scale)
		}
		return face, nil
	}

	if options.scale == 0 {
		fitStyle := style
		if options.styleSet && options.style == fonts.PixelFontAuto {
			fitStyle = fonts.PixelFontAuto
		}
		face := fonts.GetUdbFonts().FitPixelFace(text, maxWidth, maxHeight, fitStyle)
		if face.Metrics().Height.Ceil() > maxHeight {
			face.Close()
			return nil, fmt.Errorf("font does not fit the available height")
		}
		return face, nil
	}
	face := fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{Style: style, Scale: options.scale, LetterSpacing: -1})
	if face.Metrics().Height.Ceil() > maxHeight {
		face.Close()
		return nil, fmt.Errorf("fontScale %d is too large for the available height", options.scale)
	}
	return face, nil
}

func configuredStackedTextFaces(topText, bottomText string, maxWidth, maxHeight int, bottomRatio float64, lineGap int, topOptions, bottomOptions fontOptions, wrapBottom bool) (font.Face, font.Face, error) {
	if !hasFontOptions(topOptions) && !hasFontOptions(bottomOptions) {
		if wrapBottom {
			top, bottom := findWrappedStackedTextFaces(topText, bottomText, maxWidth, maxHeight, bottomRatio, lineGap)
			return top, bottom, nil
		}
		top, bottom := findStackedTextFaces(topText, bottomText, maxWidth, maxHeight, bottomRatio, lineGap)
		return top, bottom, nil
	}
	if topOptions.scale > maxHeight || bottomOptions.scale > maxHeight {
		return nil, nil, fmt.Errorf("fontScale is too large for the available height")
	}

	topStyle := configuredStyle(topText, maxWidth, maxHeight, topOptions)
	bottomStyle := configuredStyle(bottomText, maxWidth, maxHeight, bottomOptions)
	minScale := 1
	maxScale := maxHeight
	if topOptions.scale > 0 {
		minScale, maxScale = topOptions.scale, topOptions.scale
	}

	var bestTop, bestBottom font.Face
	bestTopScale := 0
	for topScale := minScale; topScale <= maxScale; topScale++ {
		bottomScale := bottomOptions.scale
		if bottomScale == 0 {
			bottomScale = roundedScale(float64(topScale), bottomRatio)
		}
		if bottomScale > maxHeight {
			return nil, nil, fmt.Errorf("secondary font scale is too large for the available height")
		}
		top := faceAtScale(topStyle, topScale)
		bottom := faceAtScale(bottomStyle, bottomScale)
		topWidth := font.MeasureString(top, topText).Ceil()
		bottomLines := 1
		if wrapBottom {
			bottomLines = len(wrapTextLines(bottom, bottomText, maxWidth))
		}
		topHeight := top.Metrics().Height.Ceil()
		bottomHeight := bottom.Metrics().Height.Ceil()
		totalHeight := topHeight + lineGap + bottomLines*(bottomHeight+lineGap)
		if !wrapBottom {
			totalHeight = topHeight + lineGap + bottomHeight
		}
		fitsWidth := topOptions.scale > 0 || topWidth <= maxWidth
		ratioFits := bottomOptions.scaleSet || bottomRatio >= 1 || bottomHeight <= topHeight
		if fitsWidth && totalHeight <= maxHeight && ratioFits {
			if topScale >= bestTopScale {
				if bestTop != nil {
					bestTop.Close()
					bestBottom.Close()
				}
				bestTop, bestBottom, bestTopScale = top, bottom, topScale
			} else {
				top.Close()
				bottom.Close()
			}
		} else {
			top.Close()
			bottom.Close()
		}
		if topOptions.scale > 0 || topHeight > maxHeight {
			break
		}
	}
	if bestTop == nil {
		return nil, nil, fmt.Errorf("font settings do not fit the available height")
	}
	return bestTop, bestBottom, nil
}

func configuredStyle(text string, maxWidth, maxHeight int, options fontOptions) fonts.PixelFontStyle {
	style := options.style
	if !options.styleSet || style == fonts.PixelFontAuto {
		return fonts.PixelFontStyleForTextBounds(text, maxWidth, maxHeight)
	}
	return style
}

func faceAtScale(style fonts.PixelFontStyle, scale int) font.Face {
	if style == outlinedFontStyle {
		return newOutlinedFace(scale)
	}
	return fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{Style: style, Scale: scale, LetterSpacing: -1})
}

type outlinedFace struct{ scale int }

var outlinedGlyphs = map[rune][]string{
	'0': {"11111", "10001", "10011", "10101", "11001", "10001", "11111"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"11111", "00001", "00001", "11111", "10000", "10000", "11111"},
	'3': {"11111", "00001", "00001", "01111", "00001", "00001", "11111"},
	'4': {"10001", "10001", "10001", "11111", "00001", "00001", "00001"},
	'5': {"11111", "10000", "10000", "11111", "00001", "00001", "11111"},
	'6': {"11111", "10000", "10000", "11111", "10001", "10001", "11111"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"11111", "10001", "10001", "11111", "10001", "10001", "11111"},
	'9': {"11111", "10001", "10001", "11111", "00001", "00001", "11111"},
	':': {" ", "1", "1", " ", "1", "1", " "},
	'.': {" ", " ", " ", " ", " ", "1", "1"},
	'-': {"     ", "     ", "     ", "11111", "     ", "     ", "     "},
	' ': {"   ", "   ", "   ", "   ", "   ", "   ", "   "},
	'?': {"11111", "00001", "00010", "00100", "00100", "     ", "00100"},
}

func newOutlinedFace(scale int) *outlinedFace { return &outlinedFace{scale: scale} }

func fitOutlinedFace(text string, maxWidth, maxHeight int) font.Face {
	scale := 1
	for candidate := 1; candidate <= maxHeight; candidate++ {
		face := newOutlinedFace(candidate)
		if font.MeasureString(face, text).Ceil() > maxWidth || face.Metrics().Height.Ceil() > maxHeight {
			break
		}
		scale = candidate
	}
	return newOutlinedFace(scale)
}

func (f *outlinedFace) glyph(r rune) []string {
	if r >= 'a' && r <= 'z' {
		r -= 'a' - 'A'
	}
	if glyph, ok := outlinedGlyphs[r]; ok {
		return glyph
	}
	return outlinedGlyphs['?']
}

func (f *outlinedFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	rows := f.glyph(r)
	width := 1
	for _, row := range rows {
		width = max(width, len(row))
	}
	mask := image.NewAlpha(image.Rect(0, 0, width*f.scale, len(rows)*f.scale))
	for y, row := range rows {
		for x, bit := range row {
			if bit != '1' {
				continue
			}
			for sy := y * f.scale; sy < (y+1)*f.scale; sy++ {
				for sx := x * f.scale; sx < (x+1)*f.scale; sx++ {
					mask.SetAlpha(sx, sy, color.Alpha{A: 255})
				}
			}
		}
	}
	x, y := dot.X.Ceil(), dot.Y.Ceil()-len(rows)*f.scale
	advance := (width + 1) * f.scale
	return image.Rect(x, y, x+width*f.scale, y+len(rows)*f.scale), mask, image.Point{}, fixed.I(advance), true
}

func (f *outlinedFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	width := 1
	for _, row := range f.glyph(r) {
		width = max(width, len(row))
	}
	height := len(f.glyph(r)) * f.scale
	return fixed.R(0, -height, width*f.scale, 0), fixed.I((width + 1) * f.scale), true
}

func (f *outlinedFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	_, advance, ok := f.GlyphBounds(r)
	return advance, ok
}

func (f *outlinedFace) Kern(r0, r1 rune) fixed.Int26_6 { return 0 }
func (f *outlinedFace) Metrics() font.Metrics {
	return font.Metrics{Height: fixed.I(7 * f.scale), Ascent: fixed.I(7 * f.scale)}
}
func (f *outlinedFace) Close() error { return nil }

func roundedScale(scale, ratio float64) int { return max(1, int(math.Round(scale*ratio))) }
