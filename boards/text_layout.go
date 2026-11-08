package boards

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/benwiebe/udb-plugin-library/fonts"
	"github.com/benwiebe/udb-plugin-library/types"
	"golang.org/x/image/font"
)

func resolveInnerRect(dimensions types.BoardDimensions, padding *int) image.Rectangle {
	marginX := int(float64(dimensions.Width) * 0.10)
	marginY := int(float64(dimensions.Height) * 0.10)
	if padding != nil {
		marginX = *padding
		marginY = *padding
	}

	maxMarginX := dimensions.Width / 2
	maxMarginY := dimensions.Height / 2
	if marginX > maxMarginX {
		marginX = maxMarginX
	}
	if marginY > maxMarginY {
		marginY = maxMarginY
	}

	return image.Rect(marginX, marginY, dimensions.Width-marginX, dimensions.Height-marginY)
}

func findTextFace(text string, maxWidth, maxHeight int) font.Face {
	face := fonts.GetUdbFonts().FitPixelFace(text, maxWidth, maxHeight, fonts.PixelFontAuto)
	if font.MeasureString(face, text).Ceil() > maxWidth || face.Metrics().Height.Ceil() > maxHeight {
		face.Close()
		face = fonts.GetUdbFonts().FitPixelFace(text, maxWidth, maxHeight, fonts.PixelFontCondensed)
	}
	return face
}

func validateBoardDimensions(dimensions types.BoardDimensions, padding *int) error {
	if dimensions.Width <= 0 || dimensions.Height <= 0 {
		return fmt.Errorf("display dimensions must be positive")
	}
	if padding != nil && *padding < 0 {
		return fmt.Errorf("padding must be >= 0")
	}
	inner := resolveInnerRect(dimensions, padding)
	if inner.Dx() < 3 || inner.Dy() < 5 {
		return fmt.Errorf("display and padding must leave at least 3x5 pixels for text")
	}
	return nil
}

func truncateText(face font.Face, text string, maxWidth int) string {
	if font.MeasureString(face, text).Ceil() <= maxWidth {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		if shortened := string(runes) + "..."; font.MeasureString(face, shortened).Ceil() <= maxWidth {
			return shortened
		}
	}
	if font.MeasureString(face, "...").Ceil() <= maxWidth {
		return "..."
	}
	return ""
}

// Month names, weekdays and unpadded days vary in length. Size against a full
// leap year rather than assuming Go's reference date is the widest output.
func widestTimeSample(layout string, uppercase bool, location *time.Location) string {
	if location == nil {
		location = time.Local
	}
	face := fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{Style: fonts.PixelFontDotMatrix, Scale: 1, LetterSpacing: -1})
	defer face.Close()
	widest, width := "", -1
	for day := 0; day < 366; day++ {
		date := time.Date(2028, 1, 1+day, 23, 59, 59, 0, location)
		for _, hour := range []int{0, 12, 23} {
			text := date.Add(time.Duration(hour-23) * time.Hour).Format(layout)
			if uppercase {
				text = strings.ToUpper(text)
			}
			if measured := font.MeasureString(face, text).Ceil(); measured > width {
				widest, width = text, measured
			}
		}
	}
	return widest
}

func findStackedTextFaces(topText, bottomText string, maxWidth, maxHeight int, bottomScale float64, lineGap int) (font.Face, font.Face) {
	if bottomScale <= 0 {
		bottomScale = 0.5
	}

	bestTopScale := 1
	bestBottomScale := 1
	topStyle := fonts.PixelFontStyleForTextBounds(topText, maxWidth, maxHeight)
	bottomStyle := fonts.PixelFontStyleForTextBounds(bottomText, maxWidth, maxHeight)
	for topScale := 1; topScale <= maxHeight; topScale++ {
		bottomScalePixels := int(math.Round(float64(topScale) * bottomScale))
		if bottomScalePixels < 1 {
			bottomScalePixels = 1
		}

		topFace := fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{
			Style:         topStyle,
			Scale:         topScale,
			LetterSpacing: -1,
		})
		bottomFace := fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{
			Style:         bottomStyle,
			Scale:         bottomScalePixels,
			LetterSpacing: -1,
		})

		topWidth := font.MeasureString(topFace, topText).Ceil()
		bottomWidth := font.MeasureString(bottomFace, bottomText).Ceil()

		topMetrics := topFace.Metrics()
		bottomMetrics := bottomFace.Metrics()
		topHeight := (topMetrics.Ascent + topMetrics.Descent).Ceil()
		bottomHeight := (bottomMetrics.Ascent + bottomMetrics.Descent).Ceil()

		topFace.Close()
		bottomFace.Close()

		totalHeight := topHeight + lineGap + bottomHeight
		maxLineWidth := topWidth
		if bottomWidth > maxLineWidth {
			maxLineWidth = bottomWidth
		}

		if maxLineWidth > maxWidth || totalHeight > maxHeight {
			break
		}

		bestTopScale = topScale
		bestBottomScale = bottomScalePixels
	}

	topFace := fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{
		Style:         topStyle,
		Scale:         bestTopScale,
		LetterSpacing: -1,
	})
	bottomFace := fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{
		Style:         bottomStyle,
		Scale:         bestBottomScale,
		LetterSpacing: -1,
	})
	if topFace.Metrics().Height.Ceil()+bottomFace.Metrics().Height.Ceil()+lineGap > maxHeight {
		topFace.Close()
		bottomFace.Close()
		return findWrappedStackedTextFaces(topText, bottomText, maxWidth, maxHeight, bottomScale, lineGap)
	}
	return topFace, bottomFace
}

// Try pixel styles and integer scales together so wrapping and relative label
// size participate in the fit, rather than shrinking only the countdown later.
func findWrappedStackedTextFaces(topText, bottomText string, maxWidth, maxHeight int, bottomRatio float64, lineGap int) (font.Face, font.Face) {
	bestTop, bestBottom, bestHeight := font.Face(nil), font.Face(nil), 0
	bestRatioError := math.Inf(1)
	for _, topStyle := range []fonts.PixelFontStyle{fonts.PixelFontStyleForTextBounds(topText, maxWidth, maxHeight), fonts.PixelFontCondensed} {
		for _, bottomStyle := range []fonts.PixelFontStyle{fonts.PixelFontDotMatrix, fonts.PixelFontCondensed} {
			for scale := 1; scale <= maxHeight; scale++ {
				top := fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{Style: topStyle, Scale: scale, LetterSpacing: -1})
				labelScale := max(1, int(math.Round(float64(scale)*bottomRatio)))
				bottom := fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{Style: bottomStyle, Scale: labelScale, LetterSpacing: -1})
				topHeight, bottomHeight := top.Metrics().Height.Ceil(), bottom.Metrics().Height.Ceil()
				lines := wrapTextLines(bottom, bottomText, maxWidth)
				height := topHeight + len(lines)*(bottomHeight+lineGap)
				ratioError := math.Abs(float64(bottomHeight)/float64(topHeight) - bottomRatio)
				if font.MeasureString(top, topText).Ceil() <= maxWidth && height <= maxHeight && (bottomRatio >= 1 || bottomHeight <= topHeight) && (topHeight > bestHeight || topHeight == bestHeight && ratioError < bestRatioError) {
					if bestTop != nil {
						bestTop.Close()
						bestBottom.Close()
					}
					bestTop, bestBottom, bestHeight = top, bottom, topHeight
					bestRatioError = ratioError
				} else {
					top.Close()
					bottom.Close()
				}
				if topHeight > maxHeight {
					break
				}
			}
		}
	}
	if bestTop == nil {
		bestTop = findTextFace(topText, maxWidth, maxHeight)
		bestBottom = fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{Style: fonts.PixelFontCondensed, Scale: 1, LetterSpacing: -1})
	}
	return bestTop, bestBottom
}

func isHorizontalAlign(v string) bool {
	switch v {
	case "left", "center", "right":
		return true
	default:
		return false
	}
}

func isVerticalAlign(v string) bool {
	switch v {
	case "top", "center", "bottom":
		return true
	default:
		return false
	}
}

func alignedX(inner image.Rectangle, textWidth int, align string) int {
	switch align {
	case "left":
		return inner.Min.X
	case "right":
		return inner.Max.X - textWidth
	default:
		return inner.Min.X + (inner.Dx()-textWidth)/2
	}
}

func alignedBaselineY(inner image.Rectangle, ascent, height, descent int, align string) int {
	switch align {
	case "top":
		return inner.Min.Y + ascent
	case "bottom":
		return inner.Max.Y - descent
	default:
		return inner.Min.Y + (inner.Dy()-height)/2 + ascent
	}
}

func alignedBlockTop(inner image.Rectangle, blockHeight int, align string) int {
	switch align {
	case "top":
		return inner.Min.Y
	case "bottom":
		return inner.Max.Y - blockHeight
	default:
		return inner.Min.Y + (inner.Dy()-blockHeight)/2
	}
}

func wrapTextLines(face font.Face, text string, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	current := ""
	for _, word := range words {
		if font.MeasureString(face, word).Ceil() > maxWidth {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			lines = append(lines, wrapLongWord(face, word, maxWidth)...)
			continue
		}

		next := word
		if current != "" {
			next = current + " " + word
		}
		if font.MeasureString(face, next).Ceil() <= maxWidth {
			current = next
			continue
		}

		lines = append(lines, current)
		current = word
	}

	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func wrapLongWord(face font.Face, word string, maxWidth int) []string {
	var lines []string
	current := ""
	for len(word) > 0 {
		r, size := utf8.DecodeRuneInString(word)
		word = word[size:]
		next := current + string(r)
		if current != "" && font.MeasureString(face, next).Ceil() > maxWidth {
			lines = append(lines, current)
			current = string(r)
			continue
		}
		current = next
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
