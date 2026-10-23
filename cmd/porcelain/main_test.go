package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/danmcfan/porcelain/internal/brandkit"
)

func TestAnalyzeSongErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		exit int
		code string
	}{
		{[]string{"analyze-song"}, exitUsage, "usage"},
		{[]string{"analyze-song", "a.wav", "b.wav"}, exitUsage, "usage"},
		{[]string{"analyze-song", "no-such-song.wav"}, exitError, "invalid_song"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), tc.args, &stdout, &stderr)
		var e cliError
		if err := json.Unmarshal(stderr.Bytes(), &e); err != nil {
			t.Fatalf("%v: stderr is not JSON: %q", tc.args, stderr.String())
		}
		if code != tc.exit || e.Code != tc.code || stdout.Len() != 0 {
			t.Errorf("%v: exit %d, code %q, stdout %q", tc.args, code, e.Code, stdout.String())
		}
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"nope"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
}

func TestBrandKitSaveThenShow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORCELAIN_BRAND_KIT", filepath.Join(dir, "saved", "brandkit.json"))
	if err := os.WriteFile(filepath.Join(dir, "band.ttf"), goregular.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "kit.json")
	if err := os.WriteFile(src, []byte(`{"band_name":"Porcelain","fonts":["band.ttf"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	if code := run(ctx, []string{"brand-kit", "show"}, &stdout, &stderr); code != exitError || !strings.Contains(stderr.String(), `"no_brand_kit"`) {
		t.Fatalf("show before save: exit %d, stderr %s", code, stderr.String())
	}
	stderr.Reset()
	if code := run(ctx, []string{"brand-kit", "save", src}, &stdout, &stderr); code != exitOK {
		t.Fatalf("save: exit %d, stderr %s", code, stderr.String())
	}
	stdout.Reset()
	if code := run(ctx, []string{"brand-kit", "show"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("show: exit %d, stderr %s", code, stderr.String())
	}
	var got struct {
		BrandKit brandkit.BrandKit `json:"brand_kit"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.BrandKit.BandName != "Porcelain" || got.BrandKit.Fonts[0] != filepath.Join(dir, "band.ttf") {
		t.Errorf("show = %s", stdout.String())
	}
}
