package analysis_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/danmcfan/porcelain/internal/analysis"
	"github.com/danmcfan/porcelain/internal/ffmpeg"
	"github.com/danmcfan/porcelain/internal/ffmpeg/ffmpegtest"
	"github.com/danmcfan/porcelain/internal/probe"
)

func source(t *testing.T, name string) probe.Source {
	t.Helper()
	s := probe.One(context.Background(), filepath.Join(ffmpegtest.Fixtures(t), "footage", name))
	if s.Error != "" {
		t.Fatal(s.Error)
	}
	return s
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.1 }

func TestShotsFindsHardCut(t *testing.T) {
	src := source(t, "IMG_0006.MP4")
	shots, err := analysis.Shots(context.Background(), analysis.Cache{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 2 || shots[0].Start != 0 || !near(shots[0].End, 1.5) ||
		!near(shots[1].Start, 1.5) || !near(shots[1].End, 3) {
		t.Fatalf("shots = %+v, want [0,1.5) [1.5,3)", shots)
	}
	if shots[0].Source != src.Path {
		t.Errorf("source = %q, want %q", shots[0].Source, src.Path)
	}
}

func TestShotsSingleShotSource(t *testing.T) {
	shots, err := analysis.Shots(context.Background(), analysis.Cache{Dir: t.TempDir()}, source(t, "IMG_0003.MP4"))
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 1 || shots[0].Start != 0 || !near(shots[0].End, 2) {
		t.Fatalf("shots = %+v, want one Shot [0,2)", shots)
	}
}

func TestProxyIsUprightConstantFPSAndCached(t *testing.T) {
	ctx := context.Background()
	cache := analysis.Cache{Dir: t.TempDir()}
	src := source(t, "IMG_0001.MOV") // rotated HEVC, two audio streams
	p, err := analysis.Proxy(ctx, cache, src)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ffmpeg.Probe(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Streams) != 1 {
		t.Fatalf("proxy has %d streams, want video only", len(r.Streams))
	}
	v := r.Streams[0]
	if v.Width != 180 || v.Height != 320 || v.AvgFrameRate != "30/1" || len(v.SideData) != 0 {
		t.Errorf("proxy %dx%d @ %s side_data=%v, want upright 180x320 @ 30/1, no rotation",
			v.Width, v.Height, v.AvgFrameRate, v.SideData)
	}
	again, err := analysis.Proxy(ctx, cache, src)
	if err != nil || again != p {
		t.Errorf("second Proxy = %q, %v; want cached %q", again, err, p)
	}
}

func TestContactSheetsHaveACellRowPerShot(t *testing.T) {
	ctx := context.Background()
	cache := analysis.Cache{Dir: t.TempDir()}
	src := source(t, "IMG_0006.MP4")
	shots, err := analysis.Shots(ctx, cache, src)
	if err != nil {
		t.Fatal(err)
	}
	sheets, err := analysis.ContactSheets(ctx, cache, src, shots)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 1 {
		t.Fatalf("got %d sheets, want 1", len(sheets))
	}
	sh := sheets[0]
	if sh.Columns != analysis.SheetColumns || sh.Rows != 2 || len(sh.Cells) != 2*analysis.SheetColumns {
		t.Fatalf("sheet %+v, want %d columns x 2 rows", sh, analysis.SheetColumns)
	}
	r, err := ffmpeg.Probe(ctx, sh.Image)
	if err != nil {
		t.Fatal(err)
	}
	w, h := r.Streams[0].Width, r.Streams[0].Height
	if w != analysis.SheetColumns*analysis.CellSize || h != 2*analysis.CellSize {
		t.Errorf("image %dx%d, want %dx%d", w, h, analysis.SheetColumns*analysis.CellSize, 2*analysis.CellSize)
	}
	for _, c := range sh.Cells {
		s := shots[c.Shot]
		if c.Time < s.Start || c.Time >= s.End {
			t.Errorf("cell %+v outside its Shot %+v", c, s)
		}
	}
}
