package render_test

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
	"github.com/danmcfan/porcelain/internal/ffmpeg/ffmpegtest"
	"github.com/danmcfan/porcelain/internal/render"
	"github.com/danmcfan/porcelain/internal/timeline"
)

// effectTimeline is one moving Clip from Source second 5, to the 120 bpm
// click Song (a Beat every 0.5 s), with the given Effects.
func effectTimeline(fx string, dur float64, effects ...timeline.Effect) *timeline.Timeline {
	return &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "1:1", FPS: 30},
		Song:          timeline.Song{Path: filepath.Join(fx, "songs", "click-120bpm.wav")},
		Clips: []timeline.Clip{{Source: filepath.Join(fx, "footage", "IMG_0007.MP4"),
			In: 5, Out: 5 + dur, Effects: effects}},
	}
}

// renderFrames renders tl and returns every frame, downscaled to 64x64.
func renderFrames(t *testing.T, tl *timeline.Timeline) []image.Image {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out.mp4")
	if _, err := render.Render(context.Background(), tl, out); err != nil {
		t.Fatal(err)
	}
	pattern := filepath.Join(dir, "f%04d.png")
	if err := ffmpeg.Run(context.Background(), "-i", out, "-vf", "scale=64:64", pattern); err != nil {
		t.Fatal(err)
	}
	var imgs []image.Image
	for i := 1; ; i++ {
		f, err := os.Open(fmt.Sprintf(pattern, i))
		if os.IsNotExist(err) {
			return imgs
		}
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		imgs = append(imgs, img)
	}
}

// meanDiff is the mean absolute RGB difference of two frames, 0 to 255.
func meanDiff(a, b image.Image) float64 {
	var sum float64
	bd := a.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			sum += math.Abs(float64(r1)-float64(r2)) + math.Abs(float64(g1)-float64(g2)) + math.Abs(float64(b1)-float64(b2))
		}
	}
	return sum / float64(3*bd.Dx()*bd.Dy()) / 257
}

// maxDiff is the largest meanDiff between matching frames.
func maxDiff(t *testing.T, a, b []image.Image) float64 {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%d frames vs %d", len(a), len(b))
	}
	m := 0.0
	for i := range a {
		m = math.Max(m, meanDiff(a[i], b[i]))
	}
	return m
}

func TestRenderEachEffectChangesFramesOnlyAboveZero(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	plain := renderFrames(t, effectTimeline(fx, 1))
	for _, typ := range timeline.EffectTypes {
		t.Run(typ, func(t *testing.T) {
			if d := maxDiff(t, plain, renderFrames(t, effectTimeline(fx, 1, timeline.NewEffect(typ, 0)))); d > 1 {
				t.Errorf("intensity 0: frames differ from no Effect by %.2f, want <= 1", d)
			}
			if d := maxDiff(t, plain, renderFrames(t, effectTimeline(fx, 1, timeline.NewEffect(typ, 1)))); d < 5 {
				t.Errorf("intensity 1: frames differ from no Effect by only %.2f, want >= 5", d)
			}
		})
	}
}

func TestRenderAllEffectsTogether(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	var all []timeline.Effect
	for _, typ := range timeline.EffectTypes {
		all = append(all, timeline.NewEffect(typ, 0.7))
	}
	if n := len(renderFrames(t, effectTimeline(fx, 1, all...))); n != 30 {
		t.Errorf("%d frames, want 30", n)
	}
}

func TestRenderSpeedRampKeepsDurationAndEnds(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	plain := renderFrames(t, effectTimeline(fx, 2))
	ramp := renderFrames(t, effectTimeline(fx, 2, timeline.NewEffect(timeline.SpeedRamp, 1)))
	if len(ramp) != 60 || len(plain) != 60 {
		t.Fatalf("%d ramp frames, %d plain; want 60 (2 s at 30 fps)", len(ramp), len(plain))
	}
	// Each end matches Source In / Out within one frame.
	near := func(img image.Image, frames ...image.Image) float64 {
		m := math.Inf(1)
		for _, f := range frames {
			m = math.Min(m, meanDiff(img, f))
		}
		return m
	}
	if d := near(ramp[0], plain[0], plain[1]); d > 2 {
		t.Errorf("first frame differs from Source In by %.2f", d)
	}
	if d := near(ramp[59], plain[58], plain[59]); d > 2 {
		t.Errorf("last frame differs from Source Out by %.2f", d)
	}
}

func TestRenderStrobeFlashesOnBeats(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	plain := renderFrames(t, effectTimeline(fx, 2))
	strobe := renderFrames(t, effectTimeline(fx, 2, timeline.NewEffect(timeline.Strobe, 1)))
	var bright []int
	for i := range strobe {
		if meanDiff(plain[i], strobe[i]) > 30 {
			bright = append(bright, i)
		}
	}
	// The click Song has a Beat every 0.5 s: 0, 0.5, 1 and 1.5 s in the Clip.
	if len(bright) < 3 {
		t.Fatalf("bright frames %v, want one per Beat", bright)
	}
	for _, i := range bright {
		at := float64(i) / 30
		if d := math.Abs(at - math.Round(at*2)/2); d > 1.0/30+1e-9 {
			t.Errorf("bright frame %d (%.3f s) is %.3f s from a Beat", i, at, d)
		}
	}
}
