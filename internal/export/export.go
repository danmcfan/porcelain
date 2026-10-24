// Package export writes a Timeline in NLE formats: FCPXML for Final Cut Pro
// and OpenTimelineIO for Resolve, Premiere and others (ADR 0003). Times are
// snapped to frames: Timeline positions to the output fps, in points to the
// Source's own frame rate.
package export

import (
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/danmcfan/porcelain/internal/probe"
	"github.com/danmcfan/porcelain/internal/timeline"
)

// Sources looks up probed Footage and Songs. *probe.Media implements it.
type Sources interface {
	Source(path string) (probe.Source, error)
}

// Report describes a finished Export.
type Report struct {
	Format   string    `json:"format"`
	Clips    int       `json:"clips"`
	Duration float64   `json:"duration"`
	Dropped  []Dropped `json:"dropped"`
}

// Dropped is a Timeline feature the target format cannot represent.
type Dropped struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// dropped lists the Effects, Transitions and Text Layers no Export carries
// yet.
func dropped(t *timeline.Timeline, format string) []Dropped {
	ds := []Dropped{}
	for i, c := range t.Clips {
		for j, e := range c.Effects {
			ds = append(ds, Dropped{
				Field:  fmt.Sprintf("clips[%d].effects[%d]", i, j),
				Reason: fmt.Sprintf("Effect %q has no %s equivalent; reapply it in the editor", e.Type, format),
			})
		}
	}
	for i, c := range t.Clips {
		if c.Transition != nil {
			ds = append(ds, Dropped{
				Field:  fmt.Sprintf("clips[%d].transition", i),
				Reason: fmt.Sprintf("Transitions are not exported to %s yet; add a %q transition in the editor", format, c.Transition.Type),
			})
		}
	}
	for i := range t.TextLayers {
		ds = append(ds, Dropped{
			Field:  fmt.Sprintf("text_layers[%d]", i),
			Reason: fmt.Sprintf("Text Layers are not exported to %s yet; add titles in the editor", format),
		})
	}
	return ds
}

// rate is a frame rate as the rational Num/Den frames per second.
type rate struct{ Num, Den int }

// sourceRate turns a probed fps into an exact rate, recognising NTSC
// rates (59.94 = 60000/1001). Unknown rates fall back to fallback.
func sourceRate(fps float64, fallback rate) rate {
	switch {
	case fps <= 0:
		return fallback
	case math.Abs(fps-math.Round(fps)) < 0.01:
		return rate{int(math.Round(fps)), 1}
	}
	if n := math.Round(fps * 1.001); math.Abs(fps-n/1.001) < 0.01 {
		return rate{int(n) * 1000, 1001}
	}
	return fallback
}

// frames rounds seconds to whole frames at r.
func (r rate) frames(secs float64) int {
	return int(math.Round(secs * float64(r.Num) / float64(r.Den)))
}

// seconds returns the exact seconds of n frames at r.
func (r rate) seconds(n int) float64 { return float64(n*r.Den) / float64(r.Num) }

// fileURL turns a path into an absolute file:// URL.
func fileURL(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// name is a Source's file name without its extension.
func name(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
