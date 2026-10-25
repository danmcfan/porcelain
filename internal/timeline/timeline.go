// Package timeline defines the Timeline: the single source of truth for an
// edit. Render derives everything from it. skill/porcelain/timeline.schema.json
// mirrors these types; TestSchemaInSync keeps the two aligned.
package timeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// SchemaVersion is the Timeline format version this binary reads and writes.
const SchemaVersion = 1

// Tolerance is the slack, in seconds, allowed when comparing times.
const Tolerance = 0.001

// Timeline is one edit of Footage to a Song.
type Timeline struct {
	SchemaVersion int         `json:"schema_version"`
	Output        Output      `json:"output"`
	Song          Song        `json:"song"`
	Clips         []Clip      `json:"clips"`
	TextLayers    []TextLayer `json:"text_layers,omitempty"`
	// Notes is free text for the Harness (e.g. how the Creative Prompt was
	// interpreted). Render ignores it.
	Notes string `json:"notes,omitempty"`
}

// Output describes the Render's frame and rate.
type Output struct {
	Aspect string `json:"aspect"` // "9:16", "1:1" or "16:9"
	FPS    int    `json:"fps"`
}

// Song is the only audio in the Render.
type Song struct {
	Path string `json:"path"`
	// Start is the offset into the Song file, in seconds, at Timeline time 0.
	Start float64 `json:"start,omitempty"`
}

// Clip places Source range [In, Out) at Timeline time At.
type Clip struct {
	Source  string   `json:"source"` // path, relative to the Timeline file
	In      float64  `json:"in"`
	Out     float64  `json:"out"`
	At      float64  `json:"at"`
	Effects []Effect `json:"effects,omitempty"`
	// Transition blends the previous Clip into this one. The first Clip has
	// none.
	Transition *Transition `json:"transition,omitempty"`
	// Reframe zooms into the Source around a focus point. Without one the
	// Source covers the frame, center-cropped.
	Reframe *Reframe `json:"reframe,omitempty"`
}

// Reframe zooms a Clip into its Source around a focus point, past the crop
// that covers the frame, optionally pushing in or pulling out across the
// Clip.
type Reframe struct {
	// Zoom is the magnification over the cover crop at the Clip's first
	// frame: 1 shows the whole cover crop, 2 half its width.
	Zoom float64 `json:"zoom"`
	// X and Y place the focus point in the upright Source, from 0 (left,
	// top) to 1 (right, bottom). The view is centred on it, clamped to the
	// Source's edges.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// ZoomEnd is the zoom at the Clip's last frame, reached linearly; 0
	// holds Zoom.
	ZoomEnd float64 `json:"zoom_end,omitempty"`
}

// MaxZoom is the largest Reframe zoom.
const MaxZoom = 4.0

// Duration is the Clip's length on the Timeline.
func (c Clip) Duration() float64 { return c.Out - c.In }

// Effect is a visual treatment on one Clip (ADR 0005). Its only param is
// "intensity", from 0 (no change) to 1 (strongest).
type Effect struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
}

// Effect types, in the order Render applies them to a Clip.
const (
	SpeedRamp  = "speed_ramp"
	ColorGrade = "color_grade"
	VHS        = "vhs"
	FilmGrain  = "film_grain"
	Glitch     = "glitch"
	Strobe     = "strobe"
)

// EffectTypes lists every Effect type, in Render order.
var EffectTypes = []string{SpeedRamp, ColorGrade, VHS, FilmGrain, Glitch, Strobe}

// NewEffect returns an Effect of type typ at intensity.
func NewEffect(typ string, intensity float64) Effect {
	return Effect{Type: typ, Params: map[string]any{"intensity": intensity}}
}

// Intensity returns the Effect's intensity; call it only on a validated
// Effect.
func (e Effect) Intensity() float64 {
	v, _ := e.Params["intensity"].(float64)
	return v
}

// Transition blends one Clip into the next across Duration seconds,
// centred on the cut: Render plays Duration/2 of Source beyond the previous
// Clip's Out and before this Clip's In, so every Clip keeps its At and the
// cut stays on its Beat.
type Transition struct {
	Type     string  `json:"type"`
	Duration float64 `json:"duration"`
}

// Transition types.
const (
	Crossfade = "crossfade" // one Clip dissolves into the next
	Flash     = "flash"     // fades up to white and back down into the next Clip
	Dip       = "dip"       // fades through black
	Zoom      = "zoom"      // zooms into the outgoing Clip while the next fades in
	Pixelize  = "pixelize"  // breaks into blocks and resolves into the next Clip
	Flashbang = "flashbang" // hard cut to white on the cut, decaying into the next Clip
	// Glitch (the same name as the Effect) tears the frame into sideways
	// slices that flicker between the two Clips.
)

// TransitionTypes lists every Transition type.
var TransitionTypes = []string{Crossfade, Flash, Flashbang, Dip, Zoom, Pixelize, Glitch}

// TransitionIn returns the Duration of the Transition into Clip i, or 0.
func (t *Timeline) TransitionIn(i int) float64 {
	if i <= 0 || i >= len(t.Clips) || t.Clips[i].Transition == nil {
		return 0
	}
	return t.Clips[i].Transition.Duration
}

// TextLayer is text drawn over the Timeline in [Start, End), in a font
// file (normally one of the Brand Kit's).
type TextLayer struct {
	Text     string  `json:"text"`
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	Font     string  `json:"font"`               // TTF/OTF path, relative to the Timeline file
	Size     float64 `json:"size,omitempty"`     // pixel height; default DefaultTextSize of the frame height
	Color    string  `json:"color,omitempty"`    // "#RRGGBB"; default white
	Position string  `json:"position,omitempty"` // "top", "center", "bottom"; default center
}

// DefaultTextSize is a TextLayer's default size as a fraction of the output
// height.
const DefaultTextSize = 1.0 / 12

// Aspects maps each supported aspect ratio to its output resolution.
var Aspects = map[string][2]int{
	"9:16": {1080, 1920},
	"1:1":  {1080, 1080},
	"16:9": {1920, 1080},
}

// FPSValues lists the supported output frame rates.
var FPSValues = []int{24, 25, 30, 50, 60}

// Positions lists the valid TextLayer positions.
var Positions = []string{"top", "center", "bottom"}

// Size returns the output width and height.
func (o Output) Size() (int, int) {
	s := Aspects[o.Aspect]
	return s[0], s[1]
}

// Duration is the Timeline's length: the end of its last Clip.
func (t *Timeline) Duration() float64 {
	end := 0.0
	for _, c := range t.Clips {
		end = math.Max(end, c.At+c.Duration())
	}
	return end
}

// Load reads a Timeline, rejecting unknown fields, and resolves Source and
// Song paths relative to the file's directory.
func Load(path string) (*Timeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var t Timeline
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	t.resolve(filepath.Dir(path))
	return &t, nil
}

func (t *Timeline) resolve(dir string) {
	abs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(dir, p)
	}
	t.Song.Path = abs(t.Song.Path)
	for i := range t.Clips {
		t.Clips[i].Source = abs(t.Clips[i].Source)
	}
	for i := range t.TextLayers {
		t.TextLayers[i].Font = abs(t.TextLayers[i].Font)
	}
}
