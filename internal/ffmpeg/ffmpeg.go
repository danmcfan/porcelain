// Package ffmpeg is the only package that executes external processes.
// Every ffmpeg and ffprobe invocation in porcelain goes through here; the
// linter forbids os/exec everywhere else.
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ErrNotFound reports that ffmpeg or ffprobe is missing from PATH.
var ErrNotFound = errors.New("not found on PATH")

// Tool describes one external binary.
type Tool struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Found   bool   `json:"found"`
	Error   string `json:"error,omitempty"`
}

// Check locates ffmpeg and ffprobe and reads their versions.
func Check(ctx context.Context) []Tool {
	tools := []Tool{{Name: "ffmpeg"}, {Name: "ffprobe"}}
	for i := range tools {
		t := &tools[i]
		p, err := exec.LookPath(t.Name)
		if err != nil {
			t.Error = fmt.Sprintf("%s %v", t.Name, ErrNotFound)
			continue
		}
		t.Path, t.Found = p, true
		out, err := exec.CommandContext(ctx, p, "-version").Output()
		if err != nil {
			t.Error = err.Error()
			continue
		}
		t.Version = parseVersion(string(out))
	}
	return tools
}

// parseVersion extracts "8.1.2" from "ffmpeg version 8.1.2 Copyright ...".
func parseVersion(out string) string {
	line, _, _ := strings.Cut(out, "\n")
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == "version" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return strings.TrimSpace(line)
}

// Available reports whether both ffmpeg and ffprobe are on PATH.
func Available() bool {
	_, err1 := exec.LookPath("ffmpeg")
	_, err2 := exec.LookPath("ffprobe")
	return err1 == nil && err2 == nil
}

// Run executes ffmpeg with the given arguments. "-hide_banner -nostdin -y"
// and "-v error" are prepended. Stderr is included in the returned error.
func Run(ctx context.Context, args ...string) error {
	full := append([]string{"-hide_banner", "-nostdin", "-v", "error", "-y"}, args...)
	return run(ctx, "ffmpeg", full)
}

func run(ctx context.Context, name string, args []string) error {
	_, err := output(ctx, name, args)
	return err
}

func output(ctx context.Context, name string, args []string) ([]byte, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%s %w", name, ErrNotFound)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, p, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// ProbeResult is the subset of `ffprobe -show_streams -show_format` output
// porcelain uses.
type ProbeResult struct {
	Streams []Stream `json:"streams"`
	Format  Format   `json:"format"`
}

// Stream is one ffprobe stream.
type Stream struct {
	Index        int        `json:"index"`
	CodecName    string     `json:"codec_name"`
	CodecType    string     `json:"codec_type"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	RFrameRate   string     `json:"r_frame_rate"`
	AvgFrameRate string     `json:"avg_frame_rate"`
	Duration     string     `json:"duration"`
	SideData     []SideData `json:"side_data_list"`
	Tags         struct {
		Rotate string `json:"rotate"`
	} `json:"tags"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
}

// SideData is one entry of a stream's side_data_list.
type SideData struct {
	Type     string  `json:"side_data_type"`
	Rotation float64 `json:"rotation"`
}

// Format is ffprobe's container-level info.
type Format struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
}

// Probe runs ffprobe on path and decodes its JSON.
func Probe(ctx context.Context, path string) (*ProbeResult, error) {
	out, err := output(ctx, "ffprobe", []string{"-v", "error", "-show_streams", "-show_format", "-of", "json", path})
	if err != nil {
		return nil, err
	}
	var r ProbeResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("decode ffprobe output for %s: %w", path, err)
	}
	return &r, nil
}

// SceneChanges returns the times, in seconds, of frames in path whose scene
// score exceeds threshold (0-1), using ffmpeg's scene detection.
func SceneChanges(ctx context.Context, path string, threshold float64) ([]float64, error) {
	out, err := output(ctx, "ffmpeg", []string{
		"-hide_banner", "-nostdin", "-v", "error", "-i", path, "-map", "0:v:0",
		"-vf", fmt.Sprintf("select='gt(scene,%g)',metadata=print:file=-", threshold),
		"-f", "null", "-",
	})
	if err != nil {
		return nil, err
	}
	return parseSceneTimes(string(out)), nil
}

// parseSceneTimes reads pts_time from metadata=print lines such as
// "frame:44   pts:45045   pts_time:1.5015".
func parseSceneTimes(out string) []float64 {
	var times []float64
	for _, line := range strings.Split(out, "\n") {
		for _, f := range strings.Fields(line) {
			if v, ok := strings.CutPrefix(f, "pts_time:"); ok {
				if t, err := strconv.ParseFloat(v, 64); err == nil {
					times = append(times, t)
				}
			}
		}
	}
	return times
}
