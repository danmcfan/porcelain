// Package render turns a Timeline into an H.264/AAC MP4.
package render

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
	"github.com/danmcfan/porcelain/internal/probe"
	"github.com/danmcfan/porcelain/internal/song"
	"github.com/danmcfan/porcelain/internal/text"
	"github.com/danmcfan/porcelain/internal/timeline"
)

// ErrNotSupported marks Timeline features render cannot draw yet.
var ErrNotSupported = errors.New("not yet supported by render")

// Result describes a finished Render.
type Result struct {
	Output   string  `json:"output"`
	Duration float64 `json:"duration"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FPS      int     `json:"fps"`
	Clips    int     `json:"clips"`
}

// InvalidError carries validation Problems that stopped a Render.
type InvalidError struct{ Problems []timeline.Problem }

func (e *InvalidError) Error() string {
	msgs := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		msgs[i] = p.Error()
	}
	return "invalid timeline: " + strings.Join(msgs, "; ")
}

// Render validates t and writes it to out.
func Render(ctx context.Context, t *timeline.Timeline, out string) (*Result, error) {
	media := probe.NewMedia(ctx)
	if ps := timeline.Validate(t, media); len(ps) > 0 {
		return nil, &InvalidError{Problems: ps}
	}
	var beats []float64
	if hasEffect(t, timeline.Strobe) {
		a, err := song.Analyze(ctx, t.Song.Path)
		if err != nil {
			return nil, fmt.Errorf("strobe needs the Song's Beats: %w", err)
		}
		for _, b := range a.Beats {
			beats = append(beats, b-t.Song.Start)
		}
	}
	dir, err := os.MkdirTemp("", "porcelain-text-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	overlays, err := writeTextLayers(t, dir)
	if err != nil {
		return nil, err
	}
	args, err := Args(t, media, overlays, beats, out)
	if err != nil {
		return nil, err
	}
	if err := ffmpeg.Run(ctx, args...); err != nil {
		return nil, err
	}
	w, h := t.Output.Size()
	return &Result{Output: out, Duration: t.Duration(), Width: w, Height: h, FPS: t.Output.FPS, Clips: len(t.Clips)}, nil
}

// writeTextLayers rasterizes each Text Layer to a full-frame transparent PNG
// in dir and returns their paths, in Timeline order.
func writeTextLayers(t *timeline.Timeline, dir string) ([]string, error) {
	w, h := t.Output.Size()
	paths := make([]string, len(t.TextLayers))
	for i, l := range t.TextLayers {
		f, err := text.LoadFont(l.Font)
		if err != nil {
			return nil, fmt.Errorf("text_layers[%d]: %w", i, err)
		}
		c, err := text.ParseColor(l.Color)
		if err != nil {
			return nil, fmt.Errorf("text_layers[%d]: %w", i, err)
		}
		size := l.Size
		if size == 0 {
			size = float64(h) * timeline.DefaultTextSize
		}
		img, err := text.Rasterize(text.Layer{Font: f, Text: l.Text, Size: size, Color: c, Position: l.Position}, w, h)
		if err != nil {
			return nil, fmt.Errorf("text_layers[%d]: %w", i, err)
		}
		paths[i] = filepath.Join(dir, fmt.Sprintf("text%d.png", i))
		if err := writePNG(paths[i], img); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Args builds the ffmpeg arguments for a validated Timeline: one input per
// Clip (trimmed at the demuxer), each scaled to cover the frame,
// center-cropped, resampled to constant fps, concatenated, with the Song as
// the only audio. overlays holds one full-frame PNG per Text Layer, laid
// over the result while the layer is on screen. Each Clip's Effects are
// applied in its own chain; beats (Timeline seconds) time strobe.
func Args(t *timeline.Timeline, media *probe.Media, overlays []string, beats []float64, out string) ([]string, error) {
	w, h := t.Output.Size()
	fps := strconv.Itoa(t.Output.FPS)
	var args []string
	var graph strings.Builder
	for i, c := range t.Clips {
		src, err := media.Source(c.Source)
		if err != nil {
			return nil, fmt.Errorf("clips[%d]: %w", i, err)
		}
		dur := secs(c.Duration())
		if src.Kind == probe.KindStill {
			args = append(args, "-loop", "1", "-framerate", fps, "-t", dur, "-i", c.Source)
		} else {
			args = append(args, "-ss", secs(c.In), "-t", dur, "-i", c.Source)
		}
		// ffmpeg applies display rotation on decode, so frames arrive upright.
		clipBeats := make([]float64, len(beats))
		for j, b := range beats {
			clipBeats[j] = b - c.At
		}
		fmt.Fprintf(&graph,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,%sfps=%s,format=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS,%sformat=yuv420p[v%d];",
			i, w, h, w, h, rampFilter(c), fps, dur, effectFilters(c, clipBeats, t.Output.FPS), i)
	}
	for i := range t.Clips {
		fmt.Fprintf(&graph, "[v%d]", i)
	}
	fmt.Fprintf(&graph, "concat=n=%d:v=1:a=0[v]", len(t.Clips))

	total := secs(t.Duration())
	song := len(t.Clips)
	args = append(args, "-ss", secs(t.Song.Start), "-t", total, "-i", t.Song.Path)
	for i, l := range t.TextLayers {
		args = append(args, "-loop", "1", "-framerate", fps, "-t", total, "-i", overlays[i])
		fmt.Fprintf(&graph, ";[v][%d:v]overlay=0:0:enable='gte(t,%s)*lt(t,%s)',format=yuv420p[v]",
			song+1+i, secs(l.Start), secs(l.End))
	}
	args = append(args,
		"-filter_complex", graph.String(),
		"-map", "[v]", "-map", fmt.Sprintf("%d:a:0", song),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-r", fps,
		"-c:a", "aac", "-b:a", "192k", "-ar", "48000",
		"-t", total, "-movflags", "+faststart",
		out)
	return args, nil
}

func secs(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }
