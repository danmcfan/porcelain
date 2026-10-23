package timeline

import (
	"errors"
	"strings"
	"testing"
)

type fakeMedia map[string]float64 // path -> duration; negative = Still

func (f fakeMedia) Duration(p string) (float64, bool, error) {
	d, ok := f[p]
	if !ok {
		return 0, false, errors.New("no such file")
	}
	return d, d < 0, nil
}

func valid() *Timeline {
	return &Timeline{
		SchemaVersion: SchemaVersion,
		Output:        Output{Aspect: "9:16", FPS: 30},
		Song:          Song{Path: "song.wav"},
		Clips: []Clip{
			{Source: "a.mov", In: 1, Out: 3, At: 0},
			{Source: "still.jpg", In: 0, Out: 2, At: 2},
		},
	}
}

var media = fakeMedia{"song.wav": 8, "a.mov": 5, "still.jpg": -1}

func TestValidateOK(t *testing.T) {
	if ps := Validate(valid(), media); len(ps) != 0 {
		t.Fatalf("unexpected problems: %v", ps)
	}
}

func TestValidateProblems(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*Timeline)
		field string
		want  string
	}{
		{"schema version", func(t *Timeline) { t.SchemaVersion = 9 }, "schema_version", "must be 1"},
		{"aspect", func(t *Timeline) { t.Output.Aspect = "4:3" }, "output.aspect", "must be one of"},
		{"fps", func(t *Timeline) { t.Output.FPS = 0 }, "output.fps", "must be one of"},
		{"song missing", func(t *Timeline) { t.Song.Path = "nope.wav" }, "song.path", "not readable"},
		{"song required", func(t *Timeline) { t.Song.Path = "" }, "song.path", "required"},
		{"negative in", func(t *Timeline) { t.Clips[0].In = -1 }, "clips[0].in", "negative"},
		{"negative at", func(t *Timeline) { t.Clips[0].At = -1 }, "clips[0].at", "negative"},
		{"out before in", func(t *Timeline) { t.Clips[0].Out = 0.5 }, "clips[0].out", "greater than in"},
		{"past source end", func(t *Timeline) { t.Clips[0].Out = 6; t.Clips[1].At = 5 }, "clips[0].out", "past the end"},
		{"gap", func(t *Timeline) { t.Clips[1].At = 2.5 }, "clips[1].at", "contiguous"},
		{"no clips", func(t *Timeline) { t.Clips = nil }, "clips", "at least one"},
		{"longer than song", func(t *Timeline) { t.Song.Start = 5 }, "clips", "longer than the Song"},
		{"missing source", func(t *Timeline) { t.Clips[0].Source = "x.mov" }, "clips[0].source", "not readable"},
		{"effect type", func(t *Timeline) { t.Clips[0].Effects = []Effect{{}} }, "clips[0].effects[0].type", "required"},
		{"text end", func(t *Timeline) { t.TextLayers = []TextLayer{{Text: "hi", Start: 2, End: 1}} }, "text_layers[0].end", "greater"},
		{"text position", func(t *Timeline) { t.TextLayers = []TextLayer{{Text: "hi", End: 1, Position: "left"}} }, "text_layers[0].position", "one of"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tl := valid()
			c.edit(tl)
			ps := Validate(tl, media)
			for _, p := range ps {
				if p.Field == c.field && strings.Contains(p.Message, c.want) {
					return
				}
			}
			t.Fatalf("want problem %s ~ %q, got %v", c.field, c.want, ps)
		})
	}
}

func TestDuration(t *testing.T) {
	if d := valid().Duration(); d != 4 {
		t.Fatalf("Duration = %v, want 4", d)
	}
}
