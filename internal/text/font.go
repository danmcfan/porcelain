// Package text draws Text Layers in Go, so Render needs no ffmpeg built
// with drawtext. It knows nothing of the Timeline: callers pass plain values.
package text

import (
	"fmt"
	"image/color"
	"os"
	"strconv"

	"golang.org/x/image/font/opentype"
)

// LoadFont reads a TTF/OTF file (or the first font of a collection).
func LoadFont(path string) (*opentype.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := opentype.Parse(data)
	if err == nil {
		return f, nil
	}
	c, cerr := opentype.ParseCollection(data)
	if cerr != nil {
		return nil, fmt.Errorf("%s: not a TTF/OTF font: %w", path, err)
	}
	return c.Font(0)
}

// ParseColor parses "#RRGGBB"; empty is white.
func ParseColor(s string) (color.NRGBA, error) {
	if s == "" {
		return color.NRGBA{255, 255, 255, 255}, nil
	}
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, fmt.Errorf("%q is not #RRGGBB", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("%q is not #RRGGBB", s)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}
