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
