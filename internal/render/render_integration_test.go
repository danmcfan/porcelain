package render_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"testing"

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

func TestRenderRejectsTextLayers(t *testing.T) {
	fx := ffmpegtest.Fixtures(t)
	tl := &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "1:1", FPS: 30},
		Song:          timeline.Song{Path: filepath.Join(fx, "songs", "click-120bpm.wav")},
		Clips:         []timeline.Clip{{Source: filepath.Join(fx, "footage", "IMG_0002.MP4"), Out: 1}},
		TextLayers:    []timeline.TextLayer{{Text: "hi", End: 1}},
	}
	_, err := render.Render(context.Background(), tl, filepath.Join(t.TempDir(), "x.mp4"))
	if err == nil || !errors.Is(err, render.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
}
