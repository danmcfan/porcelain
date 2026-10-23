package probe_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danmcfan/porcelain/internal/ffmpeg/ffmpegtest"
	"github.com/danmcfan/porcelain/internal/probe"
)

func TestProbeFixtures(t *testing.T) {
	dir := filepath.Join(ffmpegtest.Fixtures(t), "footage")
	paths, err := probe.Expand([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]probe.Source{}
	for _, s := range probe.Probe(context.Background(), paths) {
		if s.Error != "" {
			t.Errorf("%s: %s", s.Path, s.Error)
		}
		got[filepath.Base(s.Path)] = s
	}
	if _, ok := got[".DS_Store"]; ok {
		t.Error(".DS_Store should be skipped")
	}
	if len(got) != 6 {
		t.Errorf("got %d Sources, want 6", len(got))
	}

	rot := got["IMG_0001.MOV"]
	if rot.Codec != "hevc" || rot.Rotation != -90 || rot.Width != 180 || rot.Height != 320 {
		t.Errorf("rotated HEVC: got codec=%s rotation=%d %dx%d, want hevc -90 180x320",
			rot.Codec, rot.Rotation, rot.Width, rot.Height)
	}
	if rot.AudioStream == nil || *rot.AudioStream != 1 {
		t.Errorf("rotated HEVC: audio_stream = %v, want 1", rot.AudioStream)
	}
	for name, fps := range map[string]float64{"IMG_0002.MP4": 30, "IMG_0003.MP4": 60, "IMG_0004.MP4": 59.94} {
		if got[name].FPS != fps {
			t.Errorf("%s: fps = %v, want %v", name, got[name].FPS, fps)
		}
	}
	if d := got["IMG_0002 2.MP4"].DuplicateOf; filepath.Base(d) != "IMG_0002.MP4" {
		t.Errorf("duplicate_of = %q, want IMG_0002.MP4", d)
	}
	if got["IMG_0002.MP4"].DuplicateOf != "" {
		t.Error("canonical copy must not be marked as a duplicate")
	}
	if s := got["IMG_0005.JPG"]; s.Kind != probe.KindStill || s.Width != 320 || s.Height != 240 {
		t.Errorf("Still: got kind=%s %dx%d", s.Kind, s.Width, s.Height)
	}
}
