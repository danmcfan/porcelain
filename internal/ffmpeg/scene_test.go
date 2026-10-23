package ffmpeg

import (
	"reflect"
	"testing"
)

func TestParseSceneTimes(t *testing.T) {
	out := "frame:0    pts:1500000 pts_time:1.5\nlavfi.scene_score=0.777742\n" +
		"frame:1    pts:3003    pts_time:2.0333\nlavfi.scene_score=0.41\n"
	if got, want := parseSceneTimes(out), []float64{1.5, 2.0333}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSceneTimes = %v, want %v", got, want)
	}
}
