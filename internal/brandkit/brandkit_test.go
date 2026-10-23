package brandkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSaveLoadResolvesPaths(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "fonts", "band.ttf"), goregular.TTF)
	write(t, filepath.Join(dir, "logo.png"), []byte("png"))
	src := filepath.Join(dir, "kit.json")
	write(t, src, []byte(`{"band_name":"Porcelain","fonts":["fonts/band.ttf"],"logo":"logo.png","colors":["#FF0080"],"default_look":"vhs"}`))

	k, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "config", "porcelain", "brandkit.json")
	if err := Save(k, dest); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dest)
	if err != nil {
		t.Fatal(err)
	}
	if got.BandName != "Porcelain" || got.DefaultLook != "vhs" || got.Colors[0] != "#FF0080" {
		t.Errorf("round trip lost fields: %+v", got)
	}
	if got.Fonts[0] != filepath.Join(dir, "fonts", "band.ttf") || got.Logo != filepath.Join(dir, "logo.png") {
		t.Errorf("paths not absolute to the source file: fonts %v, logo %q", got.Fonts, got.Logo)
	}
}

func TestLoadRejectsMissingFont(t *testing.T) {
	src := filepath.Join(t.TempDir(), "kit.json")
	write(t, src, []byte(`{"band_name":"Porcelain","fonts":["gone.ttf"]}`))
	_, err := Load(src)
	var inv *InvalidError
	if !errors.As(err, &inv) || inv.Problems[0].Field != "fonts[0]" {
		t.Fatalf("err = %v, want InvalidError on fonts[0]", err)
	}
}

func TestLoadProblems(t *testing.T) {
	for _, c := range []struct{ json, field, want string }{
		{`{"fonts":[]}`, "band_name", "required"},
		{`{"band_name":"x","fonts":[]}`, "fonts", "at least one"},
		{`{"band_name":"x","fonts":[],"colors":["red"]}`, "colors[0]", "#RRGGBB"},
		{`{"band_name":"x","fonts":[],"logo":"nope.png"}`, "logo", "not readable"},
	} {
		src := filepath.Join(t.TempDir(), "kit.json")
		write(t, src, []byte(c.json))
		_, err := Load(src)
		var inv *InvalidError
		if !errors.As(err, &inv) {
			t.Fatalf("%s: err = %v, want InvalidError", c.json, err)
		}
		found := false
		for _, p := range inv.Problems {
			found = found || p.Field == c.field && strings.Contains(p.Message, c.want)
		}
		if !found {
			t.Errorf("%s: want %s ~ %q, got %v", c.json, c.field, c.want, inv.Problems)
		}
	}
}

func TestLoadNotSaved(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "brandkit.json")); !errors.Is(err, ErrNotSaved) {
		t.Fatalf("err = %v, want ErrNotSaved", err)
	}
}
