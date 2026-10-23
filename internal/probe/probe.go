// Package probe inspects Footage and describes each Source.
package probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
)

// Kind classifies a Source.
type Kind string

// Source kinds.
const (
	KindVideo Kind = "video"
	KindStill Kind = "still"
	KindAudio Kind = "audio"
	KindOther Kind = "other"
)

// Source describes one Footage file.
type Source struct {
	Path     string `json:"path"`
	Kind     Kind   `json:"kind"`
	Codec    string `json:"codec,omitempty"`
	Width    int    `json:"width,omitempty"`  // display width, rotation applied
	Height   int    `json:"height,omitempty"` // display height, rotation applied
	Rotation int    `json:"rotation"`         // display-matrix degrees, e.g. -90
	// FPS is the nominal rate (ffprobe r_frame_rate, e.g. 60 or 59.94).
	// Phones record variable frame rate, so AvgFPS (measured) often differs
	// slightly; render resamples to the Timeline's constant fps either way.
	FPS      float64 `json:"fps,omitempty"`
	AvgFPS   float64 `json:"avg_fps,omitempty"`
	Duration float64 `json:"duration,omitempty"` // seconds
	// AudioStream is the absolute stream index of the first AAC audio
	// stream, or nil if there is none.
	AudioStream *int   `json:"audio_stream"`
	SHA256      string `json:"sha256"`
	// DuplicateOf names the canonical Source this file is byte-identical to.
	DuplicateOf string `json:"duplicate_of,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Expand turns directories and files into a sorted list of Source paths,
// skipping hidden files such as .DS_Store. Directories are walked recursively.
func Expand(args []string) ([]string, error) {
	var paths []string
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			paths = append(paths, a)
			continue
		}
		err = filepath.WalkDir(a, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			hidden := strings.HasPrefix(d.Name(), ".") && p != a
			if d.IsDir() {
				if hidden {
					return filepath.SkipDir
				}
				return nil
			}
			if !hidden && d.Type().IsRegular() {
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Probe describes every path concurrently and flags byte-identical duplicates.
// Per-file failures are reported in Source.Error rather than aborting.
func Probe(ctx context.Context, paths []string) []Source {
	out := make([]Source, len(paths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i] = One(ctx, paths[i])
			}
		}()
	}
	for i := range paths {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	markDuplicates(out)
	return out
}

// One describes a single file.
func One(ctx context.Context, path string) Source {
	s := Source{Path: path, Kind: KindOther}
	sum, err := hashFile(path)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	s.SHA256 = sum
	r, err := ffmpeg.Probe(ctx, path)
	if err != nil {
		// Not a media file ffprobe understands.
		return s
	}
	fromProbe(&s, r)
	return s
}

func fromProbe(s *Source, r *ffmpeg.ProbeResult) {
	s.Duration = parseFloat(r.Format.Duration)
	var video *ffmpeg.Stream
	for i := range r.Streams {
		st := &r.Streams[i]
		switch st.CodecType {
		case "video":
			if video == nil && st.Disposition.AttachedPic == 0 {
				video = st
			}
		case "audio":
			if s.AudioStream == nil && st.CodecName == "aac" {
				idx := st.Index
				s.AudioStream = &idx
			}
		}
	}
	switch {
	case video != nil && isImageFormat(r.Format.FormatName):
		s.Kind = KindStill
		s.Duration = 0
	case video != nil:
		s.Kind = KindVideo
	case len(r.Streams) > 0 && r.Streams[0].CodecType == "audio" || s.AudioStream != nil:
		s.Kind = KindAudio
		return
	default:
		return
	}
	s.Codec = video.CodecName
	s.Rotation = rotation(video)
	s.Width, s.Height = video.Width, video.Height
	if s.Rotation%180 != 0 {
		s.Width, s.Height = s.Height, s.Width
	}
	if s.Kind == KindVideo {
		s.FPS = ParseFPS(video.RFrameRate)
		s.AvgFPS = ParseFPS(video.AvgFrameRate)
		if s.FPS == 0 {
			s.FPS = s.AvgFPS
		}
		if d := parseFloat(video.Duration); d > 0 {
			s.Duration = d
		}
	}
}

func isImageFormat(name string) bool {
	return strings.Contains(name, "image2") || strings.HasSuffix(name, "_pipe")
}

// rotation returns the display-matrix rotation in degrees, normalised to
// (-180, 180]. Legacy "rotate" tags use the opposite sign.
func rotation(st *ffmpeg.Stream) int {
	deg := 0.0
	found := false
	for _, sd := range st.SideData {
		if sd.Type == "Display Matrix" {
			deg, found = sd.Rotation, true
			break
		}
	}
	if !found && st.Tags.Rotate != "" {
		deg = -parseFloat(st.Tags.Rotate)
	}
	r := int(math.Round(deg)) % 360
	if r <= -180 {
		r += 360
	} else if r > 180 {
		r -= 360
	}
	return r
}

// ParseFPS parses an ffprobe rational such as "30/1" or "60000/1001" and
// returns frames per second rounded to 3 decimals. Invalid or zero rates
// ("0/0") return 0.
func ParseFPS(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		return round3(parseFloat(s))
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 || n <= 0 {
		return 0
	}
	return round3(n / d)
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// markDuplicates sets DuplicateOf on byte-identical files. The canonical
// copy is the one with the shortest base name ("IMG_1.MP4" over
// "IMG_1 2.MP4"), then lexical order.
func markDuplicates(sources []Source) {
	groups := map[string][]int{}
	for i, s := range sources {
		if s.SHA256 != "" {
			groups[s.SHA256] = append(groups[s.SHA256], i)
		}
	}
	for _, idx := range groups {
		if len(idx) < 2 {
			continue
		}
		canon := idx[0]
		for _, i := range idx[1:] {
			a, b := filepath.Base(sources[i].Path), filepath.Base(sources[canon].Path)
			if len(a) < len(b) || len(a) == len(b) && a < b {
				canon = i
			}
		}
		for _, i := range idx {
			if i != canon {
				sources[i].DuplicateOf = sources[canon].Path
			}
		}
	}
}
