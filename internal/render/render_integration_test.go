package render_test

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
	"github.com/danmcfan/porcelain/internal/ffmpeg/ffmpegtest"
	"github.com/danmcfan/porcelain/internal/render"
	"github.com/danmcfan/porcelain/internal/timeline"
)

func TestRenderThreeClips(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	foot := func(n string) string { return filepath.Join(fx, "footage", n) }
	// Clips sum to the full 8 s Song.
	tl := &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "9:16", FPS: 30},
		Song:          timeline.Song{Path: filepath.Join(fx, "songs", "click-120bpm.wav")},
		Clips: []timeline.Clip{
			{Source: foot("IMG_0001.MOV"), In: 0, Out: 2, At: 0},     // rotated HEVC
			{Source: foot("IMG_0004.MP4"), In: 0.5, Out: 2, At: 2},   // 59.94 fps landscape
			{Source: foot("IMG_0005.JPG"), In: 0, Out: 4.5, At: 3.5}, // Still
		},
	}
	out := filepath.Join(t.TempDir(), "out.mp4")
	ctx := context.Background()
	if _, err := render.Render(ctx, tl, out); err != nil {
		t.Fatal(err)
	}

	r, err := ffmpeg.Probe(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	var video, audio int
	for _, s := range r.Streams {
		switch s.CodecType {
		case "video":
			video++
			if s.CodecName != "h264" || s.Width != 1080 || s.Height != 1920 {
				t.Errorf("video: %s %dx%d, want h264 1080x1920", s.CodecName, s.Width, s.Height)
			}
			if s.AvgFrameRate != "30/1" {
				t.Errorf("fps = %s, want 30/1", s.AvgFrameRate)
			}
		case "audio":
			audio++
			if s.CodecName != "aac" {
				t.Errorf("audio codec = %s, want aac", s.CodecName)
			}
		}
	}
	if video != 1 || audio != 1 {
		t.Errorf("streams: %d video, %d audio; want 1 and 1", video, audio)
	}
	var dur float64
	if _, err := fmt.Sscan(r.Format.Duration, &dur); err != nil || math.Abs(dur-8) > 0.1 {
		t.Errorf("duration = %s, want ~8 (Song length)", r.Format.Duration)
	}
}

func TestRenderDrawsTextLayersInTheirTimeRange(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	dir := t.TempDir()
	font := filepath.Join(dir, "band.ttf")
	if err := os.WriteFile(font, goregular.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	// A Still keeps the background constant, so only the text changes.
	tl := &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "1:1", FPS: 30},
		Song:          timeline.Song{Path: filepath.Join(fx, "songs", "click-120bpm.wav")},
		Clips:         []timeline.Clip{{Source: filepath.Join(fx, "footage", "IMG_0005.JPG"), Out: 2}},
		TextLayers:    []timeline.TextLayer{{Text: "TOUR", End: 1, Font: font, Size: 200, Color: "#00FF00"}},
	}
	out := filepath.Join(dir, "out.mp4")
	ctx := context.Background()
	if _, err := render.Render(ctx, tl, out); err != nil {
		t.Fatal(err)
	}
	with, without := frame(t, out, 0.5), frame(t, out, 1.5)
	changed := 0
	b := with.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r1, g1, _, _ := with.At(x, y).RGBA()
			r2, g2, _, _ := without.At(x, y).RGBA()
			if g1>>8 > 200 && r1>>8 < 80 && (g2>>8 < 150 || r2>>8 > 120) {
				changed++
			}
		}
	}
	// "TOUR" at 200 px covers well over 5000 pixels.
	if changed < 5000 {
		t.Errorf("%d green text pixels at 0.5 s that are absent at 1.5 s; want the Text Layer drawn only in [0, 1)", changed)
	}
}

func TestRenderTransitionKeepsLengthAndFlashesOnTheCut(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	// IMG_0007 is 30 s of test pattern, so the Clips have Source either
	// side of the flash at 2 s. A hard cut at 1 s comes first: the
	// Transition must join onto a concat.
	src := filepath.Join(fx, "footage", "IMG_0007.MP4")
	tl := &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "1:1", FPS: 30},
		Song:          timeline.Song{Path: filepath.Join(fx, "songs", "click-120bpm.wav")},
		Clips: []timeline.Clip{
			{Source: src, In: 5, Out: 6, At: 0},
			{Source: src, In: 10, Out: 11, At: 1},
			{Source: src, In: 15, Out: 17, At: 2, Transition: &timeline.Transition{Type: timeline.Flash, Duration: 0.8}},
		},
	}
	out := filepath.Join(t.TempDir(), "out.mp4")
	ctx := context.Background()
	res, err := render.Render(ctx, tl, out)
	if err != nil {
		t.Fatal(err)
	}
	if res.Duration != 4 {
		t.Errorf("Duration = %v, want 4: a Transition must not shorten the Timeline", res.Duration)
	}
	r, err := ffmpeg.Probe(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	var dur float64
	if _, err := fmt.Sscan(r.Format.Duration, &dur); err != nil || math.Abs(dur-4) > 0.1 {
		t.Errorf("file duration = %s, want ~4", r.Format.Duration)
	}
	// The flash peaks at white on the cut and is gone well before it.
	if cut, before := luma(frame(t, out, 2)), luma(frame(t, out, 1.2)); cut < 200 || cut < before+60 {
		t.Errorf("mean luma %0.f on the cut vs %0.f at 1.2 s; want a white flash centred on the cut", cut, before)
	}
}

// cutTimeline joins 2 s of moving test pattern to 2 s of a Still with a
// Transition of type typ, so the cut is at 2 s.
func cutTimeline(fx, typ string, d float64) *timeline.Timeline {
	return &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "1:1", FPS: 30},
		Song:          timeline.Song{Path: filepath.Join(fx, "songs", "click-120bpm.wav")},
		Clips: []timeline.Clip{
			{Source: filepath.Join(fx, "footage", "IMG_0007.MP4"), In: 5, Out: 7, At: 0},
			{Source: filepath.Join(fx, "footage", "IMG_0005.JPG"), Out: 2, At: 2, Transition: &timeline.Transition{Type: typ, Duration: d}},
		},
	}
}

func TestRenderEveryTransitionType(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	for _, typ := range timeline.TransitionTypes {
		t.Run(typ, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.mp4")
			res, err := render.Render(context.Background(), cutTimeline(fx, typ, 0.4), out)
			if err != nil {
				t.Fatal(err)
			}
			if res.Duration != 4 {
				t.Errorf("Duration = %v, want 4", res.Duration)
			}
			// Well after the Transition only the Still is left.
			if d := meanDiff(frame(t, out, 2.5), frame(t, out, 3.5)); d > 3 {
				t.Errorf("frames at 2.5 s and 3.5 s differ by %.1f; want the Transition over", d)
			}
		})
	}
}

func TestRenderFlashbangHitsWhiteOnTheCut(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	out := filepath.Join(t.TempDir(), "out.mp4")
	if _, err := render.Render(context.Background(), cutTimeline(fx, timeline.Flashbang, 0.6), out); err != nil {
		t.Fatal(err)
	}
	before, early, cut, after := luma(frame(t, out, 1.8)), luma(frame(t, out, 1)), luma(frame(t, out, 2)), luma(frame(t, out, 3))
	if before > early+20 {
		t.Errorf("mean luma %.0f just before the cut vs %.0f at 1 s; want the outgoing Clip untouched until the cut", before, early)
	}
	if cut < 230 {
		t.Errorf("mean luma %.0f on the cut; want a white hit", cut)
	}
	if mid := luma(frame(t, out, 2.15)); mid >= cut || mid <= after {
		t.Errorf("mean luma %.0f at 2.15 s; want it decaying from %.0f on the cut toward %.0f", mid, cut, after)
	}
}

func TestRenderGlitchTearsOnlyAroundTheCut(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	out := filepath.Join(t.TempDir(), "out.mp4")
	if _, err := render.Render(context.Background(), cutTimeline(fx, timeline.Glitch, 0.6), out); err != nil {
		t.Fatal(err)
	}
	still := frame(t, out, 3)
	if d := meanDiff(frame(t, out, 2), still); d < 15 {
		t.Errorf("frame on the cut differs from the incoming Still by %.1f; want it torn", d)
	}
	if d := meanDiff(frame(t, out, 2.4), still); d > 3 {
		t.Errorf("frame after the Transition differs from the Still by %.1f; want it clean", d)
	}
}

func TestRenderReframeZoomsOnTheFocusPointAndPushesIn(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	// A Still keeps the picture constant, so only the Reframe changes it.
	still := filepath.Join(fx, "footage", "IMG_0005.JPG")
	tl := &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "1:1", FPS: 30},
		Song:          timeline.Song{Path: filepath.Join(fx, "songs", "click-120bpm.wav")},
		Clips: []timeline.Clip{
			{Source: still, Out: 1, At: 0, Reframe: &timeline.Reframe{Zoom: 4, X: 0, Y: 0}},
			{Source: still, Out: 1, At: 1, Reframe: &timeline.Reframe{Zoom: 4, X: 1, Y: 1}},
			{Source: still, Out: 2, At: 2, Reframe: &timeline.Reframe{Zoom: 1, X: 0.5, Y: 0.5, ZoomEnd: 4}},
			{Source: still, Out: 1, At: 4, Reframe: &timeline.Reframe{Zoom: 4, X: 0.5, Y: 0.5}},
			{Source: still, Out: 1, At: 5},
		},
	}
	out := filepath.Join(t.TempDir(), "out.mp4")
	if _, err := render.Render(context.Background(), tl, out); err != nil {
		t.Fatal(err)
	}
	corner, opposite := frame(t, out, 0.5), frame(t, out, 1.5)
	pushStart, pushEnd := frame(t, out, 2), frame(t, out, 3.96)
	zoomed, plain := frame(t, out, 4.5), frame(t, out, 5.5)
	if d := meanDiff(corner, opposite); d < 30 {
		t.Errorf("top-left and bottom-right Reframes differ by %.0f; want different parts of the Source", d)
	}
	if d := meanDiff(pushStart, plain); d > 8 {
		t.Errorf("push-in starts %.0f away from the plain cover crop; want zoom 1 to match it", d)
	}
	if d := meanDiff(pushEnd, zoomed); d > 15 {
		t.Errorf("push-in ends %.0f away from a zoom 4 Reframe; want it to land there", d)
	}
}

func TestRenderRejectsMissingFont(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	tl := &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "1:1", FPS: 30},
		Song:          timeline.Song{Path: filepath.Join(fx, "songs", "click-120bpm.wav")},
		Clips:         []timeline.Clip{{Source: filepath.Join(fx, "footage", "IMG_0002.MP4"), Out: 1}},
		TextLayers:    []timeline.TextLayer{{Text: "hi", End: 1, Font: filepath.Join(fx, "no-such.ttf")}},
	}
	_, err := render.Render(context.Background(), tl, filepath.Join(t.TempDir(), "x.mp4"))
	var inv *render.InvalidError
	if !errors.As(err, &inv) || inv.Problems[0].Field != "text_layers[0].font" {
		t.Fatalf("err = %v, want InvalidError on text_layers[0].font", err)
	}
}

// luma is an image's mean brightness, 0 to 255.
func luma(img image.Image) float64 {
	var sum float64
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			sum += 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
		}
	}
	return sum / float64(b.Dx()*b.Dy())
}

func frame(t *testing.T, video string, at float64) image.Image {
	t.Helper()
	p := filepath.Join(t.TempDir(), "frame.png")
	if err := ffmpeg.Run(context.Background(), "-ss", fmt.Sprint(at), "-i", video, "-frames:v", "1", p); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}
