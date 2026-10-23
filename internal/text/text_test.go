package text

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

func parse(t *testing.T, data []byte) *opentype.Font {
	t.Helper()
	f, err := opentype.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// inked returns the rows spanned by visible pixels and whether all of them
// have color c.
func inked(img *image.NRGBA, c color.NRGBA) (top, bottom int, allC bool) {
	top, bottom, allC = -1, -1, true
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := img.NRGBAAt(x, y)
			if p.A == 0 {
				continue
			}
			if top < 0 {
				top = y
			}
			bottom = y
			if p.R != c.R || p.G != c.G || p.B != c.B {
				allC = false
			}
		}
	}
	return top, bottom, allC
}

func TestRasterizeDrawsInFontAndColor(t *testing.T) {
	pink := color.NRGBA{255, 0, 128, 255}
	l := Layer{Text: "PORCELAIN", Size: 100, Color: pink, Position: "center"}
	l.Font = parse(t, goregular.TTF)
	img, err := Rasterize(l, 1080, 1920)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 1080, 1920) {
		t.Fatalf("bounds = %v", img.Bounds())
	}
	top, bottom, allPink := inked(img, pink)
	if top < 0 || !allPink {
		t.Fatalf("ink rows %d-%d, all pink %v", top, bottom, allPink)
	}
	if top < 860 || bottom > 1060 || bottom-top < 50 {
		t.Errorf("centered 100px text spans rows %d-%d", top, bottom)
	}

	l.Font = parse(t, gomono.TTF)
	other, err := Rasterize(l, 1080, 1920)
	if err != nil {
		t.Fatal(err)
	}
	if string(other.Pix) == string(img.Pix) {
		t.Error("different fonts drew identical pixels: the font is ignored")
	}
}

func TestRasterizePositions(t *testing.T) {
	f := parse(t, goregular.TTF)
	white := color.NRGBA{255, 255, 255, 255}
	for _, c := range []struct {
		pos      string
		min, max int // ink must fall within these rows
	}{
		{"top", 0, 640},
		{"bottom", 1280, 1920},
	} {
		img, err := Rasterize(Layer{Font: f, Text: "Live\nTonight", Size: 90, Color: white, Position: c.pos}, 1080, 1920)
		if err != nil {
			t.Fatal(err)
		}
		top, bottom, _ := inked(img, white)
		if top < c.min || bottom > c.max || bottom-top < 150 {
			t.Errorf("%s: two lines span rows %d-%d, want within %d-%d", c.pos, top, bottom, c.min, c.max)
		}
	}
}
