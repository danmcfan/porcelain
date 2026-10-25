package timeline

import (
	"fmt"
	"math"
	"slices"

	"github.com/danmcfan/porcelain/internal/text"
)

// Problem is one validation failure, located by a JSON-path-like Field.
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (p Problem) Error() string { return p.Field + ": " + p.Message }

// Media answers questions about files on disk. Validate uses it for checks
// that need the Footage; pass nil to skip them.
type Media interface {
	// Duration returns a file's length in seconds; still is true for a
	// Still, which has no intrinsic duration. A non-nil error means the
	// file is missing or unreadable.
	Duration(path string) (seconds float64, still bool, err error)
}

// Validate checks schema and semantics and returns every Problem found.
func Validate(t *Timeline, m Media) []Problem {
	var ps []Problem
	add := func(field, format string, args ...any) {
		ps = append(ps, Problem{Field: field, Message: fmt.Sprintf(format, args...)})
	}

	if t.SchemaVersion != SchemaVersion {
		add("schema_version", "must be %d, got %d", SchemaVersion, t.SchemaVersion)
	}
	if _, ok := Aspects[t.Output.Aspect]; !ok {
		add("output.aspect", "must be one of 9:16, 1:1, 16:9, got %q", t.Output.Aspect)
	}
	if !slices.Contains(FPSValues, t.Output.FPS) {
		add("output.fps", "must be one of %v, got %d", FPSValues, t.Output.FPS)
	}

	songLen := math.Inf(1)
	if t.Song.Path == "" {
		add("song.path", "is required")
	}
	if t.Song.Start < 0 {
		add("song.start", "must not be negative")
	}
	if m != nil && t.Song.Path != "" {
		if d, _, err := m.Duration(t.Song.Path); err != nil {
			add("song.path", "Song not readable: %v", err)
		} else {
			songLen = d - t.Song.Start
		}
	}

	if len(t.Clips) == 0 {
		add("clips", "must contain at least one Clip")
	}
	expectAt := 0.0
	srcLen := make([]float64, len(t.Clips)) // +Inf for a Still or unknown length
	for i, c := range t.Clips {
		srcLen[i] = math.Inf(1)
		f := fmt.Sprintf("clips[%d]", i)
		if c.Source == "" {
			add(f+".source", "is required")
		}
		if c.In < 0 {
			add(f+".in", "must not be negative")
		}
		if c.At < 0 {
			add(f+".at", "must not be negative")
		}
		if c.Out <= c.In {
			add(f+".out", "must be greater than in (%g)", c.In)
		}
		if math.Abs(c.At-expectAt) > Tolerance {
			add(f+".at", "Clips must be contiguous in order: expected %g, got %g", expectAt, c.At)
		}
		expectAt = c.At + c.Duration()
		seen := map[string]bool{}
		for j, e := range c.Effects {
			ef := fmt.Sprintf("%s.effects[%d]", f, j)
			switch {
			case e.Type == "":
				add(ef+".type", "is required")
			case !slices.Contains(EffectTypes, e.Type):
				add(ef+".type", "must be one of %v, got %q", EffectTypes, e.Type)
			case seen[e.Type]:
				add(ef+".type", "%q is on this Clip more than once", e.Type)
			}
			seen[e.Type] = true
			for k := range e.Params {
				if k != "intensity" {
					add(ef+".params."+k, "unknown param: Effects take only intensity")
				}
			}
			switch v, ok := e.Params["intensity"]; {
			case !ok:
				add(ef+".params.intensity", "is required: a number from 0 to 1")
			case !isNumber(v):
				add(ef+".params.intensity", "must be a number, got %v", v)
			case v.(float64) < 0 || v.(float64) > 1:
				add(ef+".params.intensity", "must be between 0 and 1, got %g", v)
			}
		}
		if r := c.Reframe; r != nil {
			if r.Zoom < 1 || r.Zoom > MaxZoom {
				add(f+".reframe.zoom", "must be between 1 and %g, got %g", MaxZoom, r.Zoom)
			}
			if r.ZoomEnd != 0 && (r.ZoomEnd < 1 || r.ZoomEnd > MaxZoom) {
				add(f+".reframe.zoom_end", "must be 0 (hold zoom) or between 1 and %g, got %g", MaxZoom, r.ZoomEnd)
			}
			if r.X < 0 || r.X > 1 {
				add(f+".reframe.x", "must be between 0 and 1, got %g", r.X)
			}
			if r.Y < 0 || r.Y > 1 {
				add(f+".reframe.y", "must be between 0 and 1, got %g", r.Y)
			}
		}
		if m != nil && c.Source != "" {
			d, still, err := m.Duration(c.Source)
			switch {
			case err != nil:
				add(f+".source", "Source not readable: %v", err)
			case !still && c.Out > d+Tolerance:
				add(f+".out", "%g is past the end of the Source (%g s)", c.Out, d)
			case !still:
				srcLen[i] = d
			}
		}
	}
	for i, c := range t.Clips {
		if c.Transition == nil {
			continue
		}
		f := fmt.Sprintf("clips[%d].transition", i)
		tr := c.Transition
		if !slices.Contains(TransitionTypes, tr.Type) {
			add(f+".type", "must be one of %v, got %q", TransitionTypes, tr.Type)
		}
		if i == 0 {
			add(f, "the first Clip has no Clip before it to transition from")
			continue
		}
		if tr.Duration <= 0 {
			add(f+".duration", "must be greater than 0")
			continue
		}
		prev, half := t.Clips[i-1], tr.Duration/2
		if tr.Duration > c.Duration()+Tolerance || tr.Duration > prev.Duration()+Tolerance {
			add(f+".duration", "%g is longer than this Clip (%g s) or the one before it (%g s)", tr.Duration, c.Duration(), prev.Duration())
		}
		// Half the Transition plays Source beyond each side of the cut.
		if c.In-half < -Tolerance && !math.IsInf(srcLen[i], 1) {
			add(f+".duration", "needs %g s of Source before in (%g): move in later or shorten the Transition", half, c.In)
		}
		if prev.Out+half > srcLen[i-1]+Tolerance {
			add(f+".duration", "needs %g s of Source after clips[%d].out (%g), past the end of that Source (%g s): move it earlier or shorten the Transition", half, i-1, prev.Out, srcLen[i-1])
		}
	}
	if total := t.Duration(); total > songLen+Tolerance {
		add("clips", "Timeline (%g s) is longer than the Song from song.start (%g s)", total, songLen)
	}

	for i, l := range t.TextLayers {
		f := fmt.Sprintf("text_layers[%d]", i)
		if l.Text == "" {
			add(f+".text", "is required")
		}
		if l.Start < 0 {
			add(f+".start", "must not be negative")
		}
		if l.End <= l.Start {
			add(f+".end", "must be greater than start")
		}
		if l.Position != "" && !slices.Contains(Positions, l.Position) {
			add(f+".position", "must be one of %v", Positions)
		}
		if l.Size < 0 {
			add(f+".size", "must not be negative")
		}
		if _, err := text.ParseColor(l.Color); err != nil {
			add(f+".color", "%v", err)
		}
		switch {
		case l.Font == "":
			add(f+".font", "is required: use a Brand Kit font")
		case m != nil:
			if _, err := text.LoadFont(l.Font); err != nil {
				add(f+".font", "font not readable: %v", err)
			}
		}
	}
	return ps
}

func isNumber(v any) bool {
	_, ok := v.(float64)
	return ok
}
