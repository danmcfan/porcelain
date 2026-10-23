package export

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/danmcfan/porcelain/internal/timeline"
)

// otioObject is an OTIO JSON object: its OTIO_SCHEMA plus fields.
type otioObject map[string]any

func otioTime(r rate, n int) otioObject {
	return otioObject{"OTIO_SCHEMA": "RationalTime.1", "value": n, "rate": r.Num / r.Den}
}

func otioRange(r rate, start, duration int) otioObject {
	return otioObject{"OTIO_SCHEMA": "TimeRange.1", "start_time": otioTime(r, start), "duration": otioTime(r, duration)}
}

func otioClip(r rate, path string, start, duration int) otioObject {
	return otioObject{
		"OTIO_SCHEMA":  "Clip.2",
		"name":         name(path),
		"metadata":     otioObject{},
		"source_range": otioRange(r, start, duration),
		"effects":      []any{},
		"markers":      []any{},
		"enabled":      true,
		"media_references": otioObject{"DEFAULT_MEDIA": otioObject{
			"OTIO_SCHEMA":     "ExternalReference.1",
			"name":            "",
			"metadata":        otioObject{},
			"available_range": nil,
			"target_url":      fileURL(path),
		}},
		"active_media_reference_key": "DEFAULT_MEDIA",
	}
}

func otioTrack(kind string, children []otioObject) otioObject {
	return otioObject{
		"OTIO_SCHEMA":  "Track.1",
		"name":         kind,
		"kind":         kind,
		"metadata":     otioObject{},
		"source_range": nil,
		"effects":      []any{},
		"markers":      []any{},
		"enabled":      true,
		"children":     children,
	}
}

// OTIO writes t, already validated, as an OpenTimelineIO Timeline: a Video
// track of Clips and an Audio track holding the Song. All times are frames
// at the output fps.
func OTIO(t *timeline.Timeline, srcs Sources, w io.Writer) (*Report, error) {
	seq := rate{t.Output.FPS, 1}
	video := []otioObject{}
	for i, c := range t.Clips {
		if _, err := srcs.Source(c.Source); err != nil {
			return nil, fmt.Errorf("clips[%d]: %w", i, err)
		}
		at, end := seq.frames(c.At), seq.frames(c.At+c.Duration())
		video = append(video, otioClip(seq, c.Source, seq.frames(c.In), end-at))
	}
	total := seq.frames(t.Duration())
	audio := []otioObject{otioClip(seq, t.Song.Path, seq.frames(t.Song.Start), total)}

	doc := otioObject{
		"OTIO_SCHEMA":       "Timeline.1",
		"name":              "porcelain",
		"metadata":          otioObject{},
		"global_start_time": nil,
		"tracks": otioObject{
			"OTIO_SCHEMA":  "Stack.1",
			"name":         "tracks",
			"metadata":     otioObject{},
			"source_range": nil,
			"effects":      []any{},
			"markers":      []any{},
			"enabled":      true,
			"children":     []otioObject{otioTrack("Video", video), otioTrack("Audio", audio)},
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return &Report{Format: "otio", Clips: len(t.Clips), Duration: seq.seconds(total), Dropped: dropped(t, "OTIO")}, nil
}
