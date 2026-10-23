package text

import (
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Layer is one Text Layer's look, resolved to plain values.
type Layer struct {
	Font     *opentype.Font
	Text     string // "\n" separates lines
	Size     float64
	Color    color.NRGBA
	Position string // "top", "center" (default) or "bottom"
}

// Rasterize draws l onto a transparent w x h frame: each line centered
// horizontally, the block placed at Position with a margin of h/12 at the
// top or bottom.
func Rasterize(l Layer, w, h int) (*image.NRGBA, error) {
	face, err := opentype.NewFace(l.Font, &opentype.FaceOptions{Size: l.Size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer func() { _ = face.Close() }()

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	m := face.Metrics()
	lines := strings.Split(l.Text, "\n")
	block := m.Height.Mul(fixed.I(len(lines)-1)) + m.Ascent + m.Descent
	margin := fixed.I(h / 12)
	var top fixed.Int26_6
	switch l.Position {
	case "top":
		top = margin
	case "bottom":
		top = fixed.I(h) - margin - block
	default:
		top = (fixed.I(h) - block) / 2
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(l.Color), Face: face}
	for i, line := range lines {
		d.Dot = fixed.Point26_6{
			X: (fixed.I(w) - d.MeasureString(line)) / 2,
			Y: top + m.Ascent + m.Height.Mul(fixed.I(i)),
		}
		d.DrawString(line)
	}
	return img, nil
}
