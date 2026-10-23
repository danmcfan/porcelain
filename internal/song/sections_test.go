package song

import (
	"math"
	"testing"
)

// clickSong builds a 120 BPM click track over a drone, one bar (2 s) per
// level, each bar played at that amplitude.
func clickSong(levels []float64) []float32 {
	const barLen = 2 * rate
	out := make([]float32, len(levels)*barLen)
	for i := range out {
		t := float64(i) / rate
		v := 0.15 * math.Sin(2*math.Pi*110*t)
		if math.Mod(t, 0.5) < 0.03 {
			v += 0.6 * math.Sin(2*math.Pi*1000*t)
		}
		out[i] = float32(levels[i/barLen] * v)
	}
	return out
}

// repeat returns n copies of level.
func repeat(level float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = level
	}
	return out
}

func TestSectionsFollowEnergy(t *testing.T) {
	type want struct {
		label string
		start float64
	}
	for _, tc := range []struct {
		name   string
		levels [][]float64
		want   []want
	}{
		{
			"quiet-loud-quiet",
			[][]float64{repeat(0.1, 4), repeat(0.8, 8), repeat(0.1, 4)},
			[]want{{"intro", 0}, {"chorus", 8}, {"outro", 24}},
		},
		{
			"verse-chorus twice",
			[][]float64{repeat(0.25, 4), repeat(0.8, 4), repeat(0.25, 4), repeat(0.8, 4)},
			[]want{{"verse", 0}, {"chorus", 8}, {"verse", 16}, {"chorus", 24}},
		},
		{
			"full song",
			[][]float64{
				repeat(0.06, 2), repeat(0.25, 4), repeat(0.8, 4), repeat(0.25, 4),
				repeat(0.8, 4), repeat(0.45, 4), repeat(0.8, 4), repeat(0.06, 2),
			},
			[]want{
				{"intro", 0}, {"verse", 4}, {"chorus", 12}, {"verse", 20},
				{"chorus", 28}, {"bridge", 36}, {"chorus", 44}, {"outro", 52},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var levels []float64
			for _, l := range tc.levels {
				levels = append(levels, l...)
			}
			a := analyzePCM(clickSong(levels))
			if len(a.Sections) != len(tc.want) {
				t.Fatalf("got %d Sections, want %d: %+v", len(a.Sections), len(tc.want), a.Sections)
			}
			for i, s := range a.Sections {
				w := tc.want[i]
				if s.Label != w.label || math.Abs(s.Start-w.start) > 1.0/30 {
					t.Errorf("Section %d = %s at %.3f, want %s at %v", i, s.Label, s.Start, w.label, w.start)
				}
				if i > 0 && s.Start != a.Sections[i-1].End {
					t.Errorf("Section %d starts at %v, previous ends at %v", i, s.Start, a.Sections[i-1].End)
				}
			}
			if end := a.Sections[len(a.Sections)-1].End; end != a.Duration {
				t.Errorf("last Section ends at %v, want Duration %v", end, a.Duration)
			}
		})
	}
}
