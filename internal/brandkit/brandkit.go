// Package brandkit stores the band's Brand Kit: saved once per user and
// copied by the Skill into every new Timeline (Text Layer fonts, colors,
// default Look) unless the Creative Prompt overrides it.
package brandkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/danmcfan/porcelain/internal/text"
	"github.com/danmcfan/porcelain/internal/timeline"
)

// ErrNotSaved means no Brand Kit file exists yet.
var ErrNotSaved = errors.New("no Brand Kit saved")

// BrandKit is the band's identity. Paths are absolute once loaded.
type BrandKit struct {
	BandName    string   `json:"band_name"`
	Fonts       []string `json:"fonts"`            // TTF/OTF files; the first is the default
	Logo        string   `json:"logo,omitempty"`   // image file
	Colors      []string `json:"colors,omitempty"` // "#RRGGBB"
	DefaultLook string   `json:"default_look,omitempty"`
}

// InvalidError carries the Problems that make a Brand Kit unusable.
type InvalidError struct{ Problems []timeline.Problem }

func (e *InvalidError) Error() string {
	msgs := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		msgs[i] = p.Error()
	}
	return "invalid Brand Kit: " + strings.Join(msgs, "; ")
}

// DefaultPath is where the Brand Kit is saved: $PORCELAIN_BRAND_KIT, or
// brandkit.json in porcelain's user config directory.
func DefaultPath() (string, error) {
	if p := os.Getenv("PORCELAIN_BRAND_KIT"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "porcelain", "brandkit.json"), nil
}

// Load reads a Brand Kit, rejecting unknown fields, resolves its paths
// relative to the file, and checks that every font and the logo exist.
func Load(path string) (*BrandKit, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", path, ErrNotSaved)
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var k BrandKit
	if err := dec.Decode(&k); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	abs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(dir, p)
	}
	for i := range k.Fonts {
		k.Fonts[i] = abs(k.Fonts[i])
	}
	k.Logo = abs(k.Logo)
	if ps := validate(&k); len(ps) > 0 {
		return nil, &InvalidError{Problems: ps}
	}
	return &k, nil
}

// Save validates k and writes it to path, creating its directory.
func Save(k *BrandKit, path string) error {
	if ps := validate(k); len(ps) > 0 {
		return &InvalidError{Problems: ps}
	}
	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func validate(k *BrandKit) []timeline.Problem {
	var ps []timeline.Problem
	add := func(field, format string, args ...any) {
		ps = append(ps, timeline.Problem{Field: field, Message: fmt.Sprintf(format, args...)})
	}
	if k.BandName == "" {
		add("band_name", "is required")
	}
	if len(k.Fonts) == 0 {
		add("fonts", "must contain at least one TTF/OTF font")
	}
	for i, f := range k.Fonts {
		if _, err := text.LoadFont(f); err != nil {
			add(fmt.Sprintf("fonts[%d]", i), "font not readable: %v", err)
		}
	}
	if k.Logo != "" {
		if _, err := os.Stat(k.Logo); err != nil {
			add("logo", "not readable: %v", err)
		}
	}
	for i, c := range k.Colors {
		if _, err := text.ParseColor(c); err != nil {
			add(fmt.Sprintf("colors[%d]", i), "%v", err)
		}
	}
	return ps
}
