package export_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/danmcfan/porcelain/internal/export"
	"github.com/danmcfan/porcelain/internal/probe"
	"github.com/danmcfan/porcelain/internal/timeline"
)

// fakeSources answers Source lookups from a fixed table.
type fakeSources map[string]probe.Source

func (f fakeSources) Source(path string) (probe.Source, error) {
	s, ok := f[path]
	if !ok {
		return probe.Source{}, errors.New("no such Source")
	}
	return s, nil
}

var sources = fakeSources{
	"/f/a.mov":    {Path: "/f/a.mov", Kind: probe.KindVideo, FPS: 59.94, Duration: 10, Width: 1080, Height: 1920},
	"/f/b.mp4":    {Path: "/f/b.mp4", Kind: probe.KindVideo, FPS: 30, Duration: 5, Width: 1920, Height: 1080},
	"/f/c.jpg":    {Path: "/f/c.jpg", Kind: probe.KindStill, Width: 4032, Height: 3024},
	"/s/song.wav": {Path: "/s/song.wav", Kind: probe.KindAudio, Duration: 60},
}

func sample() *timeline.Timeline {
	return &timeline.Timeline{
		SchemaVersion: timeline.SchemaVersion,
		Output:        timeline.Output{Aspect: "9:16", FPS: 30},
		Song:          timeline.Song{Path: "/s/song.wav", Start: 1.5},
		Clips: []timeline.Clip{
			{Source: "/f/a.mov", In: 0.5, Out: 2.5, At: 0},
			{Source: "/f/b.mp4", In: 1, Out: 2, At: 2},
			{Source: "/f/c.jpg", In: 0, Out: 1.5, At: 3},
		},
	}
}

type xClip struct {
	XMLName  xml.Name
	Ref      string  `xml:"ref,attr"`
	Offset   string  `xml:"offset,attr"`
	Start    string  `xml:"start,attr"`
	Duration string  `xml:"duration,attr"`
	Lane     string  `xml:"lane,attr"`
	Enable   string  `xml:"srcEnable,attr"`
	Children []xClip `xml:",any"`
}

type xFCPXML struct {
	Version   string `xml:"version,attr"`
	Resources struct {
		Formats []struct {
			ID            string `xml:"id,attr"`
			FrameDuration string `xml:"frameDuration,attr"`
			Width         string `xml:"width,attr"`
			Height        string `xml:"height,attr"`
		} `xml:"format"`
		Assets []struct {
			ID     string `xml:"id,attr"`
			Format string `xml:"format,attr"`
			Rep    struct {
				Src string `xml:"src,attr"`
			} `xml:"media-rep"`
		} `xml:"asset"`
	} `xml:"resources"`
	Sequence struct {
		Format   string `xml:"format,attr"`
		Duration string `xml:"duration,attr"`
		Spine    struct {
			Items []xClip `xml:",any"`
		} `xml:"spine"`
	} `xml:"library>event>project>sequence"`
}

func TestFCPXMLPlacesClipsAtTheirInPointsAndPositions(t *testing.T) {
	var buf bytes.Buffer
	rep, err := export.FCPXML(sample(), sources, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Dropped) != 0 {
		t.Errorf("dropped = %v, want none", rep.Dropped)
	}
	var doc xFCPXML
	if err := xml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("not XML: %v\n%s", err, buf.String())
	}
	if doc.Sequence.Duration != "135/30s" {
		t.Errorf("sequence duration = %s, want 135/30s (4.5 s)", doc.Sequence.Duration)
	}
	spine := doc.Sequence.Spine.Items
	if len(spine) != 3 {
		t.Fatalf("spine has %d items, want 3:\n%s", len(spine), buf.String())
	}
	want := []struct{ elem, offset, start, duration string }{
		// a.mov is 59.94 fps: 0.5 s in = 30 frames of 1001/60000.
		{"asset-clip", "0s", "30030/60000s", "60/30s"},
		{"asset-clip", "60/30s", "30/30s", "30/30s"},
		{"clip", "90/30s", "0s", "45/30s"},
	}
	for i, w := range want {
		c := spine[i]
		if c.XMLName.Local != w.elem || c.Offset != w.offset || c.Start != w.start || c.Duration != w.duration {
			t.Errorf("spine[%d] = <%s offset=%s start=%s duration=%s>, want <%s offset=%s start=%s duration=%s>",
				i, c.XMLName.Local, c.Offset, c.Start, c.Duration, w.elem, w.offset, w.start, w.duration)
		}
	}
	// A Still is a <clip> wrapping a <video> of its asset.
	var videos []xClip
	for _, c := range spine[2].Children {
		if c.XMLName.Local == "video" {
			videos = append(videos, c)
		}
	}
	if len(videos) != 1 || videos[0].Ref == "" || videos[0].Duration != "45/30s" {
		t.Errorf("Still clip videos = %+v, want one lasting 45/30s", videos)
	}
	if spine[0].Enable != "video" {
		t.Errorf("Source audio not muted: srcEnable=%q", spine[0].Enable)
	}

	// The Song rides under the first Clip on lane -1, from song.start, for
	// the whole Timeline.
	var song *xClip
	for i, c := range spine[0].Children {
		if c.Lane == "-1" {
			song = &spine[0].Children[i]
		}
	}
	if song == nil {
		t.Fatalf("no Song on lane -1:\n%s", buf.String())
	}
	if song.Offset != "30030/60000s" || song.Start != "45/30s" || song.Duration != "135/30s" {
		t.Errorf("Song offset=%s start=%s duration=%s, want 30030/60000s 45/30s 135/30s", song.Offset, song.Start, song.Duration)
	}
}

func TestFCPXMLFormatsCarryUprightSourceSize(t *testing.T) {
	// a.mov is portrait phone Footage, b.mp4 landscape; c.jpg is a Still,
	// which takes no format.
	var buf bytes.Buffer
	if _, err := export.FCPXML(sample(), sources, &buf); err != nil {
		t.Fatal(err)
	}
	var doc xFCPXML
	if err := xml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	size := map[string]string{}
	for _, f := range doc.Resources.Formats {
		size[f.ID] = f.Width + "x" + f.Height
	}
	want := []string{"1080x1920", "1920x1080", ""}
	for i, w := range want {
		a := doc.Resources.Assets[i]
		if got := size[a.Format]; got != w {
			t.Errorf("asset %s format size = %s, want %s", a.ID, got, w)
		}
	}
}

func TestFCPXMLReportsDroppedEffectsTransitionsAndTextLayers(t *testing.T) {
	tl := sample()
	tl.Clips[1].Effects = []timeline.Effect{{Type: "vhs"}}
	tl.Clips[1].Transition = &timeline.Transition{Type: timeline.Crossfade, Duration: 0.5}
	tl.TextLayers = []timeline.TextLayer{{Text: "TOUR", Start: 0, End: 1}}
	rep, err := export.FCPXML(tl, sources, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var fields []string
	for _, d := range rep.Dropped {
		fields = append(fields, d.Field)
	}
	if want := []string{"clips[1].effects[0]", "clips[1].transition", "text_layers[0]"}; !slices.Equal(fields, want) {
		t.Errorf("dropped fields = %v, want %v", fields, want)
	}
}

type oTime struct {
	Value float64 `json:"value"`
	Rate  float64 `json:"rate"`
}

type oClip struct {
	Schema      string `json:"OTIO_SCHEMA"`
	SourceRange struct {
		Start    oTime `json:"start_time"`
		Duration oTime `json:"duration"`
	} `json:"source_range"`
	Refs map[string]struct {
		Target string `json:"target_url"`
	} `json:"media_references"`
}

func TestOTIOPlacesClipsOnAVideoTrackAndTheSongOnAnAudioTrack(t *testing.T) {
	var buf bytes.Buffer
	rep, err := export.OTIO(sample(), sources, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "otio" || rep.Duration != 4.5 {
		t.Errorf("report = %+v", rep)
	}
	var doc struct {
		Schema string `json:"OTIO_SCHEMA"`
		Tracks struct {
			Children []struct {
				Kind     string  `json:"kind"`
				Children []oClip `json:"children"`
			} `json:"children"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if doc.Schema != "Timeline.1" || len(doc.Tracks.Children) != 2 {
		t.Fatalf("want a Timeline.1 with 2 tracks:\n%s", buf.String())
	}
	video, audio := doc.Tracks.Children[0], doc.Tracks.Children[1]
	if video.Kind != "Video" || audio.Kind != "Audio" {
		t.Errorf("track kinds = %s, %s", video.Kind, audio.Kind)
	}
	// Clips are contiguous, so a track's children sit back to back.
	want := []struct {
		target          string
		start, duration float64
	}{
		{"file:///f/a.mov", 15, 60},
		{"file:///f/b.mp4", 30, 30},
		{"file:///f/c.jpg", 0, 45},
	}
	if len(video.Children) != len(want) {
		t.Fatalf("video track has %d children, want %d", len(video.Children), len(want))
	}
	for i, w := range want {
		c := video.Children[i]
		r := c.SourceRange
		if c.Refs["DEFAULT_MEDIA"].Target != w.target || r.Start.Value != w.start || r.Duration.Value != w.duration || r.Start.Rate != 30 {
			t.Errorf("video[%d] = %s %+v, want %s start %g duration %g at 30 fps", i, c.Refs["DEFAULT_MEDIA"].Target, r, w.target, w.start, w.duration)
		}
	}
	if len(audio.Children) != 1 {
		t.Fatalf("audio track has %d children, want 1", len(audio.Children))
	}
	if r := audio.Children[0].SourceRange; r.Start.Value != 45 || r.Duration.Value != 135 {
		t.Errorf("Song range = %+v, want start 45 duration 135", r)
	}
}
