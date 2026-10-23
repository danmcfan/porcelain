package song_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/danmcfan/porcelain/internal/ffmpeg/ffmpegtest"
	"github.com/danmcfan/porcelain/internal/song"
)

// frame is one video frame at 60 fps, the highest output.fps: the FR-2
// tolerance for Beats.
const frame = 1.0 / 60

func TestAnalyzeClickSongs(t *testing.T) {
	dir := filepath.Join(ffmpegtest.Fixtures(t), "songs")
	for _, tc := range []struct {
		file     string
		bpm      float64
		duration float64
	}{
		{"click-120bpm.wav", 120, 8},
		{"click-96bpm-16s.wav", 96, 16},
	} {
		t.Run(tc.file, func(t *testing.T) {
			a, err := song.Analyze(context.Background(), filepath.Join(dir, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(a.Tempo-tc.bpm) > 1 {
				t.Errorf("tempo = %.2f, want %v", a.Tempo, tc.bpm)
			}
			// The clicks start at 0 and repeat every 60/bpm seconds.
			period := 60 / tc.bpm
			n := int(math.Round(tc.duration / period))
			if len(a.Beats) != n {
				t.Fatalf("got %d Beats, want %d: %v", len(a.Beats), n, a.Beats)
			}
			for i, b := range a.Beats {
				if want := float64(i) * period; math.Abs(b-want) > frame {
					t.Errorf("Beat %d at %.4f, want %.4f ± one frame", i, b, want)
				}
			}
			// Uniform clicks have no accent, so the bar starts on the first Beat.
			for i, d := range a.Downbeats {
				if d != a.Beats[4*i] {
					t.Errorf("downbeat %d at %.4f, want Beat %d at %.4f", i, d, 4*i, a.Beats[4*i])
				}
			}
			if len(a.Downbeats) != (n+3)/4 {
				t.Errorf("got %d downbeats, want %d", len(a.Downbeats), (n+3)/4)
			}
			// Flat energy: one Section covering the whole Song.
			if len(a.Sections) != 1 || a.Sections[0].Start != 0 || math.Abs(a.Sections[0].End-tc.duration) > frame {
				t.Errorf("Sections = %+v, want one spanning the Song", a.Sections)
			}
		})
	}
}
