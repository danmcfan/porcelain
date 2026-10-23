package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/danmcfan/porcelain/internal/ffmpeg/ffmpegtest"
)

func runJSON(t *testing.T, v any, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code != exitOK {
		t.Fatalf("%v: exit %d: %s", args, code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), v); err != nil {
		t.Fatalf("%v: stdout is not JSON: %q", args, stdout.String())
	}
}

type skip struct{ Source, Reason string }

func TestShotsCommandCoversFootage(t *testing.T) {
	t.Setenv("PORCELAIN_CACHE", t.TempDir())
	var res struct {
		Shots []struct {
			Source     string
			Start, End float64
		}
		Skipped []skip
	}
	runJSON(t, &res, "shots", filepath.Join(ffmpegtest.Fixtures(t), "footage"))

	perSource := map[string]int{}
	for _, s := range res.Shots {
		perSource[filepath.Base(s.Source)]++
	}
	want := map[string]int{"IMG_0001.MOV": 1, "IMG_0002.MP4": 1, "IMG_0003.MP4": 1, "IMG_0004.MP4": 1, "IMG_0006.MP4": 2, "IMG_0007.MP4": 1}
	for name, n := range want {
		if perSource[name] != n {
			t.Errorf("%s: %d Shots, want %d", name, perSource[name], n)
		}
	}
	skipped := map[string]bool{}
	for _, s := range res.Skipped {
		skipped[filepath.Base(s.Source)] = true
	}
	if !skipped["IMG_0002 2.MP4"] || !skipped["IMG_0005.JPG"] || len(perSource) != len(want) {
		t.Errorf("duplicate and Still should be skipped: shots=%v skipped=%v", perSource, res.Skipped)
	}
}

func TestContactSheetCommandWritesImages(t *testing.T) {
	t.Setenv("PORCELAIN_CACHE", t.TempDir())
	var res struct {
		ContactSheets []struct {
			Source, Image string
			Rows          int
		} `json:"contact_sheets"`
	}
	runJSON(t, &res, "contact-sheet", filepath.Join(ffmpegtest.Fixtures(t), "footage", "IMG_0006.MP4"))
	if len(res.ContactSheets) != 1 || res.ContactSheets[0].Rows != 2 {
		t.Fatalf("contact_sheets = %+v, want one sheet with 2 rows", res.ContactSheets)
	}
	if _, err := os.Stat(res.ContactSheets[0].Image); err != nil {
		t.Error(err)
	}
}

func TestAnalyzeSongCommand(t *testing.T) {
	var res struct {
		Tempo     float64
		Beats     []float64
		Downbeats []float64
		Bars      []struct{ Start, End, Energy float64 }
		Sections  []struct {
			Label      string
			Start, End float64
		}
	}
	runJSON(t, &res, "analyze-song", filepath.Join(ffmpegtest.Fixtures(t), "songs", "click-120bpm.wav"))
	if res.Tempo < 119 || res.Tempo > 121 || len(res.Beats) != 16 || len(res.Downbeats) != 4 ||
		len(res.Bars) != 4 || len(res.Sections) != 1 {
		t.Errorf("analyze-song = %+v", res)
	}
}
