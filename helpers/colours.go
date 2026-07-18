package helpers

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// parseHexColour parses a "#RRGGBB" or "RRGGBB" hex string into a color.RGBA.
func ParseHexColour(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid hex colour %q: expected 6 hex digits", s)
	}
	r, err := strconv.ParseUint(s[0:2], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid hex colour %q: %w", s, err)
	}
	g, err := strconv.ParseUint(s[2:4], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid hex colour %q: %w", s, err)
	}
	b2, err := strconv.ParseUint(s[4:6], 16, 8)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid hex colour %q: %w", s, err)
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b2), A: 255}, nil
}
