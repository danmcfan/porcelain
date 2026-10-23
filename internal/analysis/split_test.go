package analysis

import (
	"reflect"
	"testing"
)

func TestSplitDropsCutsTooCloseToABoundary(t *testing.T) {
	// 0.1 is too near the start, 2.1 too near the previous cut, 3.9 too near the end.
	got := split("a.mov", []float64{0.1, 2, 2.1, 3.9}, 4)
	want := []Shot{{"a.mov", 0, 2}, {"a.mov", 2, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("split = %+v, want %+v", got, want)
	}
}

func TestSplitShortSourceIsOneShot(t *testing.T) {
	got := split("a.mov", []float64{0.3}, 0.4)
	if want := []Shot{{"a.mov", 0, 0.4}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("split = %+v, want %+v", got, want)
	}
}
