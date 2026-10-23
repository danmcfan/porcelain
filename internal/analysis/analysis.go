// Package analysis looks at Footage for the Harness: it makes Proxies, splits
// Sources into Shots, and draws Contact Sheets. All work runs on Proxies,
// cached by Source content hash, so re-analysing unchanged Footage is fast.
package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
	"github.com/danmcfan/porcelain/internal/probe"
)

// Analysis settings. Bump cacheVersion when any of these or the Contact
// Sheet layout change, so stale cached results are not reused.
const (
	cacheVersion = "v1"
	proxyFPS     = 30
	proxyBox     = 640  // longest displayed side, in pixels
	sceneLimit   = 0.3  // ffmpeg scene score that counts as a cut
	minShot      = 0.25 // seconds
)

// Contact Sheet layout.
const (
	SheetColumns = 4   // frames sampled per Shot
	SheetRows    = 8   // Shots per Contact Sheet image
	CellSize     = 240 // square cell, frames letterboxed into it
)

// ErrNotVideo reports a Source that has no Shots (a Still, audio, or other).
var ErrNotVideo = errors.New("not a video Source")

// Cache is the directory holding Proxies, Shots and Contact Sheets, one
// subdirectory per Source content hash.
type Cache struct{ Dir string }

// DefaultCache uses $PORCELAIN_CACHE, else the user cache directory.
func DefaultCache() (Cache, error) {
	if d := os.Getenv("PORCELAIN_CACHE"); d != "" {
		return Cache{Dir: d}, nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return Cache{}, fmt.Errorf("locate cache directory (set PORCELAIN_CACHE): %w", err)
	}
	return Cache{Dir: filepath.Join(d, "porcelain")}, nil
}

func (c Cache) dir(src probe.Source) (string, error) {
	if src.SHA256 == "" {
		return "", fmt.Errorf("%s: no content hash", src.Path)
	}
	d := filepath.Join(c.Dir, src.SHA256+"-"+cacheVersion)
	return d, os.MkdirAll(d, 0o755)
}

// Shot is a continuous segment of a Source, in Source seconds.
type Shot struct {
	Source string  `json:"source"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
}

// Proxy returns the path of src's Proxy, transcoding it on first use: video
// only, rotation applied, scaled to fit proxyBox, constant proxyFPS.
func Proxy(ctx context.Context, cache Cache, src probe.Source) (string, error) {
	if src.Kind != probe.KindVideo {
		return "", fmt.Errorf("%s: %w", src.Path, ErrNotVideo)
	}
	d, err := cache.dir(src)
	if err != nil {
		return "", err
	}
	out := filepath.Join(d, "proxy.mp4")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	// ffmpeg applies display rotation on decode; the output carries none.
	vf := fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2,fps=%d,format=yuv420p",
		proxyBox, proxyBox, proxyFPS)
	err = atomically(out, func(tmp string) error {
		return ffmpeg.Run(ctx, "-i", src.Path, "-map", "0:v:0", "-an", "-sn", "-dn",
			"-vf", vf, "-c:v", "libx264", "-preset", "ultrafast", "-crf", "28", tmp)
	})
	if err != nil {
		return "", fmt.Errorf("proxy %s: %w", src.Path, err)
	}
	return out, nil
}

// Shots splits src at scene changes detected on its Proxy.
func Shots(ctx context.Context, cache Cache, src probe.Source) ([]Shot, error) {
	p, err := Proxy(ctx, cache, src)
	if err != nil {
		return nil, err
	}
	cached := filepath.Join(filepath.Dir(p), "shots.json")
	var cuts []float64
	if b, err := os.ReadFile(cached); err == nil && json.Unmarshal(b, &cuts) == nil {
		return split(src.Path, cuts, src.Duration), nil
	}
	cuts, err = ffmpeg.SceneChanges(ctx, p, sceneLimit)
	if err != nil {
		return nil, fmt.Errorf("shots %s: %w", src.Path, err)
	}
	b, err := json.Marshal(cuts)
	if err != nil {
		return nil, err
	}
	if err := atomically(cached, func(tmp string) error { return os.WriteFile(tmp, b, 0o644) }); err != nil {
		return nil, err
	}
	return split(src.Path, cuts, src.Duration), nil
}

// split turns cut times into Shots covering [0, duration]. Cuts closer than
// minShot to the previous boundary or the end are dropped, so every Shot is
// at least minShot long unless the whole Source is shorter.
func split(path string, cuts []float64, duration float64) []Shot {
	shots := []Shot{{Source: path}}
	for _, c := range cuts {
		c = round3(c)
		last := &shots[len(shots)-1]
		if c-last.Start < minShot || duration-c < minShot {
			continue
		}
		last.End = c
		shots = append(shots, Shot{Source: path, Start: c})
	}
	shots[len(shots)-1].End = round3(duration)
	return shots
}

// ContactSheet is one grid image: a row per Shot, SheetColumns frames each.
type ContactSheet struct {
	Source  string `json:"source"`
	Image   string `json:"image"`
	Columns int    `json:"columns"`
	Rows    int    `json:"rows"`
	Cells   []Cell `json:"cells"` // row-major, left to right
}

// Cell is one frame on a Contact Sheet.
type Cell struct {
	Shot int     `json:"shot"` // index into the Source's Shots
	Time float64 `json:"time"` // Source seconds
}

// ContactSheets draws src's Shots onto one or more images, SheetRows Shots
// per image, sampling SheetColumns evenly spaced frames from each Shot.
func ContactSheets(ctx context.Context, cache Cache, src probe.Source, shots []Shot) ([]ContactSheet, error) {
	p, err := Proxy(ctx, cache, src)
	if err != nil {
		return nil, err
	}
	var sheets []ContactSheet
	for first := 0; first < len(shots); first += SheetRows {
		last := min(first+SheetRows, len(shots))
		sheet := ContactSheet{
			Source:  src.Path,
			Image:   filepath.Join(filepath.Dir(p), fmt.Sprintf("sheet-%d.jpg", len(sheets)+1)),
			Columns: SheetColumns,
			Rows:    last - first,
		}
		for i := first; i < last; i++ {
			sheet.Cells = append(sheet.Cells, sample(i, shots[i], src.Duration)...)
		}
		if _, err := os.Stat(sheet.Image); err != nil {
			if err := drawSheet(ctx, p, sheet); err != nil {
				return nil, fmt.Errorf("contact sheet %s: %w", src.Path, err)
			}
		}
		sheets = append(sheets, sheet)
	}
	return sheets, nil
}

// sample picks SheetColumns times at the centres of equal slices of s,
// kept clear of the Source's last frame so a seek always finds one.
func sample(index int, s Shot, duration float64) []Cell {
	limit := math.Max(0, duration-2.0/proxyFPS)
	cells := make([]Cell, SheetColumns)
	for k := range cells {
		t := s.Start + (float64(k)+0.5)/SheetColumns*(s.End-s.Start)
		cells[k] = Cell{Shot: index, Time: round3(math.Min(t, limit))}
	}
	return cells
}

// drawSheet seeks the Proxy once per Cell, letterboxes each frame into a
// CellSize square, and tiles them.
func drawSheet(ctx context.Context, proxy string, sheet ContactSheet) error {
	var args []string
	var graph strings.Builder
	for i, c := range sheet.Cells {
		args = append(args, "-ss", strconv.FormatFloat(c.Time, 'f', 3, 64), "-i", proxy)
		fmt.Fprintf(&graph,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,trim=end_frame=1,setpts=PTS-STARTPTS[c%d];",
			i, CellSize, CellSize, CellSize, CellSize, i)
	}
	for i := range sheet.Cells {
		fmt.Fprintf(&graph, "[c%d]", i)
	}
	fmt.Fprintf(&graph, "concat=n=%d:v=1:a=0,tile=%dx%d[s]", len(sheet.Cells), sheet.Columns, sheet.Rows)
	args = append(args, "-filter_complex", graph.String(), "-map", "[s]", "-frames:v", "1", "-q:v", "3")
	return atomically(sheet.Image, func(tmp string) error {
		return ffmpeg.Run(ctx, append(args, tmp)...)
	})
}

// atomically runs write against a temporary sibling of path (same extension,
// so ffmpeg picks the right muxer) and renames it into place on success.
func atomically(path string, write func(tmp string) error) error {
	ext := filepath.Ext(path)
	tmp := strings.TrimSuffix(path, ext) + fmt.Sprintf(".tmp%d", os.Getpid()) + ext
	if err := write(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
