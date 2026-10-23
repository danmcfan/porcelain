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
	for i, c := range t.Clips {
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
		for j, e := range c.Effects {
			if e.Type == "" {
				add(fmt.Sprintf("%s.effects[%d].type", f, j), "is required")
			}
		}
		if m != nil && c.Source != "" {
			d, still, err := m.Duration(c.Source)
			switch {
			case err != nil:
				add(f+".source", "Source not readable: %v", err)
			case !still && c.Out > d+Tolerance:
				add(f+".out", "%g is past the end of the Source (%g s)", c.Out, d)
			}
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
