// Package render turns a Timeline into an H.264/AAC MP4.
package render

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
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
	lens := make([]float64, len(t.Clips))
	for i, c := range t.Clips {
		// A Transition plays half its length of Source past each side of
		// the cut, so stretch the Clip to cover it.
		pre, post := t.TransitionIn(i)/2, t.TransitionIn(i+1)/2
		start := c.At - pre
		c.In, c.Out = c.In-pre, c.Out+post
		lens[i] = c.Duration()
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
			clipBeats[j] = b - start
		}
		fit, zoom := frameFilters(c, src.Width, src.Height, w, h, t.Output.FPS)
		fmt.Fprintf(&graph,
			"[%d:v]%s,setsar=1,%s%sformat=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS,%sformat=yuv420p,settb=1/%d[v%d];",
			i, fit, resampleFilter(c, t.Output.FPS), zoom, dur, effectFilters(c, i, clipBeats, t.Output.FPS), t.Output.FPS, i)
	}
	joinClips(&graph, t, lens)

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

// frameFilters returns the filters that fit c's Source (upright
// sw x sh) to the w x h frame: fit before the fps resample, zoom (empty,
// or ending in a comma) after it. Without a Reframe the Source is scaled to
// cover the frame and center-cropped. A Reframe crops the view at its zoom
// around the focus point, in Source pixels, then scales it to the frame.
// A push-in or pull-out crops the view at the smaller zoom and zoompans
// inside it, one output frame per resampled frame, so timing is unchanged.
func frameFilters(c timeline.Clip, sw, sh, w, h, fps int) (fit, zoom string) {
	r := c.Reframe
	if r == nil {
		return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", w, h, w, h), ""
	}
	end := r.ZoomEnd
	if end == 0 {
		end = r.Zoom
	}
	cover := math.Max(float64(w)/float64(sw), float64(h)/float64(sh))
	cx, cy := r.X*float64(sw), r.Y*float64(sh)
	// view is the Source rectangle shown at zoom z, centred on the focus
	// point and clamped inside the Source.
	view := func(z float64) (vx, vy, vw, vh float64) {
		vw = math.Min(float64(w)/(cover*z), float64(sw))
		vh = math.Min(float64(h)/(cover*z), float64(sh))
		return clamp(cx-vw/2, 0, float64(sw)-vw), clamp(cy-vh/2, 0, float64(sh)-vh), vw, vh
	}
	lo := math.Min(r.Zoom, end)
	vx, vy, vw, vh := view(lo)
	fit = fmt.Sprintf("crop=%d:%d:%d:%d,scale=%d:%d", even(vw), even(vh), int(vx), int(vy), w, h)
	if end == r.Zoom {
		return fit, ""
	}
	// Inside the cropped view zoompan's zoom is relative to lo; it is
	// scaled to w x h, so place the smaller view in output pixels.
	k := float64(w) / vw
	frames := c.Duration() * float64(fps)
	z := fmt.Sprintf("(%g+%g*min(on/%g,1))", r.Zoom/lo, (end-r.Zoom)/lo, math.Max(frames-1, 1))
	zoom = fmt.Sprintf("zoompan=z='%s':x='clip(%g-iw/zoom/2,%g,%g-iw/zoom)-%g':y='clip(%g-ih/zoom/2,%g,%g-ih/zoom)-%g':d=1:s=%dx%d:fps=%d,",
		z, cx*k, 0.0, float64(sw)*k, vx*k, cy*k, 0.0, float64(sh)*k, vy*k, w, h, fps)
	return fit, zoom
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(v, hi)) }

// even rounds a crop size down to an even number of pixels, as yuv420p
// needs.
func even(f float64) int { return int(f) &^ 1 }

// joinClips writes the filters that join the Clip streams [v0], [v1], ...
// into [v]: a Transition becomes an xfade over the overlap, other cuts a
// concat. lens are the stretched Clip lengths.
func joinClips(graph *strings.Builder, t *timeline.Timeline, lens []float64) {
	if len(t.Clips) == 1 {
		graph.WriteString("[v0]null[v]")
		return
	}
	in, length := "[v0]", lens[0]
	for i := 1; i < len(t.Clips); i++ {
		out := fmt.Sprintf("[j%d];", i)
		if i == len(t.Clips)-1 {
			out = "[v]"
		}
		if d := t.TransitionIn(i); d > 0 {
			fmt.Fprintf(graph, "%s[v%d]xfade=transition=%s:duration=%s:offset=%s%s",
				in, i, xfadeTransitions[t.Clips[i].Transition.Type], secs(d), secs(length-d), out)
			length += lens[i] - d
		} else {
			// concat outputs a microsecond timebase; xfade needs both
			// inputs on the Clips' 1/fps.
			fmt.Fprintf(graph, "%s[v%d]concat=n=2:v=1:a=0,settb=1/%d%s", in, i, t.Output.FPS, out)
			length += lens[i]
		}
		in = fmt.Sprintf("[j%d]", i)
	}
}

func secs(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }
