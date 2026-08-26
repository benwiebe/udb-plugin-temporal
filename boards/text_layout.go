package boards

import (
	"image"
	"math"

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
	return fonts.GetUdbFonts().FitPixelFace(text, maxWidth, maxHeight, fonts.PixelFontAuto)
}

func findStackedTextFaces(topText, bottomText string, maxWidth, maxHeight int, bottomScale float64, lineGap int) (font.Face, font.Face) {
	if bottomScale <= 0 {
		bottomScale = 0.5
	}

	bestTopScale := 1
	bestBottomScale := 1
	topStyle := fonts.PixelFontStyleForText(topText)
	bottomStyle := fonts.PixelFontStyleForText(bottomText)
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

	return fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{
			Style:         topStyle,
			Scale:         bestTopScale,
			LetterSpacing: -1,
		}),
		fonts.GetUdbFonts().PixelFace(fonts.PixelFaceOptions{
			Style:         bottomStyle,
			Scale:         bestBottomScale,
			LetterSpacing: -1,
		})
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
