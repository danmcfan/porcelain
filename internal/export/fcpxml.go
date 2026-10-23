package export

import (
	"encoding/xml"
	"fmt"
	"io"

	"github.com/danmcfan/porcelain/internal/probe"
	"github.com/danmcfan/porcelain/internal/timeline"
)

// fcpxmlVersion is the oldest FCPXML with <media-rep>: Final Cut Pro 10.5+.
const fcpxmlVersion = "1.9"

type fcpxml struct {
	XMLName xml.Name    `xml:"fcpxml"`
	Version string      `xml:"version,attr"`
	Formats []fcpFormat `xml:"resources>format"`
	Assets  []fcpAsset  `xml:"resources>asset"`
	Event   fcpEvent    `xml:"library>event"`
}

type fcpFormat struct {
	ID            string `xml:"id,attr"`
	Name          string `xml:"name,attr,omitempty"`
	FrameDuration string `xml:"frameDuration,attr,omitempty"`
	Width         int    `xml:"width,attr,omitempty"`
	Height        int    `xml:"height,attr,omitempty"`
}

type fcpAsset struct {
	ID       string `xml:"id,attr"`
	Name     string `xml:"name,attr"`
	Start    string `xml:"start,attr"`
	Duration string `xml:"duration,attr"`
	HasVideo int    `xml:"hasVideo,attr,omitempty"`
	HasAudio int    `xml:"hasAudio,attr,omitempty"`
	Format   string `xml:"format,attr,omitempty"`
	Rep      struct {
		Kind string `xml:"kind,attr"`
		Src  string `xml:"src,attr"`
	} `xml:"media-rep"`
}

type fcpEvent struct {
	Name    string     `xml:"name,attr"`
	Project fcpProject `xml:"project"`
}

type fcpProject struct {
	Name     string      `xml:"name,attr"`
	Sequence fcpSequence `xml:"sequence"`
}

type fcpSequence struct {
	Format      string    `xml:"format,attr"`
	Duration    string    `xml:"duration,attr"`
	TCStart     string    `xml:"tcStart,attr"`
	TCFormat    string    `xml:"tcFormat,attr"`
	AudioLayout string    `xml:"audioLayout,attr"`
	AudioRate   string    `xml:"audioRate,attr"`
	Spine       []fcpClip `xml:"spine>clip"`
}

// fcpClip is an <asset-clip> (video or Song) or a <video> (Still).
type fcpClip struct {
	XMLName   xml.Name
	Ref       string      `xml:"ref,attr"`
	Lane      string      `xml:"lane,attr,omitempty"`
	Offset    string      `xml:"offset,attr"`
	Name      string      `xml:"name,attr"`
	Start     string      `xml:"start,attr"`
	Duration  string      `xml:"duration,attr"`
	SrcEnable string      `xml:"srcEnable,attr,omitempty"`
	Conform   *fcpConform `xml:"adjust-conform"`
	Connected []fcpClip   `xml:"asset-clip"`
}

type fcpConform struct {
	Type string `xml:"type,attr"`
}

// t formats n frames at r as an FCPXML rational time, e.g. "60/30s".
func (r rate) t(n int) string {
	if n == 0 {
		return "0s"
	}
	return fmt.Sprintf("%d/%ds", n*r.Den, r.Num)
}

// FCPXML writes t, already validated, as an FCPXML project: one Clip per
// spine item, Source audio disabled, the Song connected below on lane -1.
func FCPXML(t *timeline.Timeline, srcs Sources, w io.Writer) (*Report, error) {
	seq := rate{t.Output.FPS, 1}
	width, height := t.Output.Size()
	doc := fcpxml{
		Version: fcpxmlVersion,
		Formats: []fcpFormat{{ID: "r1", FrameDuration: seq.t(1), Width: width, Height: height}},
	}
	nextID := func() string { return fmt.Sprintf("r%d", len(doc.Formats)+len(doc.Assets)+1) }
	formats := map[string]string{} // "frameDuration WxH" (or "still WxH") -> id
	format := func(key string, f fcpFormat) string {
		if id, ok := formats[key]; ok {
			return id
		}
		f.ID = nextID()
		doc.Formats = append(doc.Formats, f)
		formats[key] = f.ID
		return f.ID
	}
	assets := map[string]string{} // path -> id
	asset := func(src probe.Source, r rate) string {
		if id, ok := assets[src.Path]; ok {
			return id
		}
		a := fcpAsset{Name: name(src.Path), Start: "0s", Duration: r.t(int(src.Duration*float64(r.Num)/float64(r.Den) + 0.999))}
		switch src.Kind {
		case probe.KindStill:
			a.HasVideo, a.Duration = 1, "0s"
			a.Format = format(fmt.Sprintf("still %dx%d", src.Width, src.Height), fcpFormat{Name: "FFVideoFormatRateUndefined", Width: src.Width, Height: src.Height})
		case probe.KindVideo:
			a.HasVideo, a.HasAudio = 1, boolInt(src.AudioStream != nil)
			// Width and height are upright (rotation applied), so portrait
			// phone Footage is not mistaken for landscape.
			f := fcpFormat{FrameDuration: r.t(1), Width: src.Width, Height: src.Height}
			a.Format = format(fmt.Sprintf("%s %dx%d", f.FrameDuration, f.Width, f.Height), f)
		default:
			a.HasAudio = 1
		}
		a.Rep.Kind, a.Rep.Src = "original-media", fileURL(src.Path)
		a.ID = nextID()
		doc.Assets = append(doc.Assets, a)
		assets[src.Path] = a.ID
		return a.ID
	}

	var spine []fcpClip
	for i, c := range t.Clips {
		src, err := srcs.Source(c.Source)
		if err != nil {
			return nil, fmt.Errorf("clips[%d]: %w", i, err)
		}
		at, end := seq.frames(c.At), seq.frames(c.At+c.Duration())
		item := fcpClip{
			Offset:   seq.t(at),
			Name:     name(c.Source),
			Duration: seq.t(end - at),
			Conform:  &fcpConform{Type: "fill"},
		}
		if src.Kind == probe.KindStill {
			item.XMLName.Local, item.Start = "video", "0s"
			item.Ref = asset(src, seq)
		} else {
			r := sourceRate(src.FPS, seq)
			item.XMLName.Local, item.Start, item.SrcEnable = "asset-clip", r.t(r.frames(c.In)), "video"
			item.Ref = asset(src, r)
		}
		spine = append(spine, item)
	}

	total := seq.frames(t.Duration())
	if len(spine) > 0 {
		song, err := srcs.Source(t.Song.Path)
		if err != nil {
			return nil, fmt.Errorf("song: %w", err)
		}
		// A connected clip's offset is in its parent's Source time.
		spine[0].Connected = []fcpClip{{
			XMLName:   xml.Name{Local: "asset-clip"},
			Ref:       asset(song, seq),
			Lane:      "-1",
			Offset:    spine[0].Start,
			Name:      name(t.Song.Path),
			Start:     seq.t(seq.frames(t.Song.Start)),
			Duration:  seq.t(total),
			SrcEnable: "audio",
		}}
	}
	doc.Event = fcpEvent{Name: "porcelain", Project: fcpProject{Name: "porcelain", Sequence: fcpSequence{
		Format: "r1", Duration: seq.t(total), TCStart: "0s", TCFormat: "NDF",
		AudioLayout: "stereo", AudioRate: "48k", Spine: spine,
	}}}

	if _, err := io.WriteString(w, xml.Header+"<!DOCTYPE fcpxml>\n"); err != nil {
		return nil, err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return nil, err
	}
	return &Report{Format: "fcpxml", Clips: len(t.Clips), Duration: seq.seconds(total), Dropped: dropped(t, "FCPXML")}, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
