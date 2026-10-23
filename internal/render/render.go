// Package render turns a Timeline into an H.264/AAC MP4.
package render

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
	"github.com/danmcfan/porcelain/internal/probe"
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
	if len(t.TextLayers) > 0 {
		return nil, fmt.Errorf("text_layers: %w", ErrNotSupported)
	}
	for i, c := range t.Clips {
		if len(c.Effects) > 0 {
			return nil, fmt.Errorf("clips[%d].effects: %w", i, ErrNotSupported)
		}
	}
	args, err := Args(t, media, out)
	if err != nil {
		return nil, err
	}
	if err := ffmpeg.Run(ctx, args...); err != nil {
		return nil, err
	}
	w, h := t.Output.Size()
	return &Result{Output: out, Duration: t.Duration(), Width: w, Height: h, FPS: t.Output.FPS, Clips: len(t.Clips)}, nil
}

// Args builds the ffmpeg arguments for a validated Timeline: one input per
// Clip (trimmed at the demuxer), each scaled to cover the frame,
// center-cropped, resampled to constant fps, concatenated, with the Song as
// the only audio.
func Args(t *timeline.Timeline, media *probe.Media, out string) ([]string, error) {
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
		fmt.Fprintf(&graph,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,fps=%s,format=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS[v%d];",
			i, w, h, w, h, fps, dur, i)
	}
	for i := range t.Clips {
		fmt.Fprintf(&graph, "[v%d]", i)
	}
	fmt.Fprintf(&graph, "concat=n=%d:v=1:a=0[v]", len(t.Clips))

	total := secs(t.Duration())
	song := len(t.Clips)
	args = append(args, "-ss", secs(t.Song.Start), "-t", total, "-i", t.Song.Path)
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
