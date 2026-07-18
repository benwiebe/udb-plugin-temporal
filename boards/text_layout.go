package boards

import (
	"image"

	"github.com/benwiebe/udb-plugin-library/fonts"
	"github.com/benwiebe/udb-plugin-library/types"
	"github.com/golang/freetype/truetype"
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

// findTextFontSize binary-searches for the largest font size where text fits within maxWidth x maxHeight.
func findTextFontSize(text string, maxWidth, maxHeight int) float64 {
	lo, hi := 1.0, float64(maxHeight)*2
	for hi-lo > 0.5 {
		mid := (lo + hi) / 2
		face := truetype.NewFace(
			fonts.GetUdbFonts().FontForSize(int(mid)), &truetype.Options{Size: mid, DPI: 72})
		d := font.Drawer{Face: face}
		w := d.MeasureString(text).Ceil()
		m := face.Metrics()
		h := (m.Ascent + m.Descent).Ceil()
		face.Close()
		if w <= maxWidth && h <= maxHeight {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

func findStackedTextFontSize(topText, bottomText string, maxWidth, maxHeight int, bottomScale float64, lineGap int) float64 {
	if bottomScale <= 0 {
		bottomScale = 0.5
	}

	lo, hi := 1.0, float64(maxHeight)*2
	for hi-lo > 0.5 {
		mid := (lo + hi) / 2
		topFace := truetype.NewFace(
			fonts.GetUdbFonts().FontForSize(int(mid)), &truetype.Options{Size: mid, DPI: 72})
		bottomFace := truetype.NewFace(
			fonts.GetUdbFonts().FontForSize(int(mid*bottomScale)), &truetype.Options{Size: mid * bottomScale, DPI: 72})

		topDrawer := font.Drawer{Face: topFace}
		bottomDrawer := font.Drawer{Face: bottomFace}

		topWidth := topDrawer.MeasureString(topText).Ceil()
		bottomWidth := bottomDrawer.MeasureString(bottomText).Ceil()

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

		if maxLineWidth <= maxWidth && totalHeight <= maxHeight {
			lo = mid
		} else {
			hi = mid
		}
	}

	return lo
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
