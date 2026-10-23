package probe

import "testing"

func TestParseFPS(t *testing.T) {
	cases := map[string]float64{
		"30/1":        30,
		"60/1":        60,
		"60000/1001":  59.94,
		"30000/1001":  29.97,
		"24000/1001":  23.976,
		"165600/2761": 59.978,
		"0/0":         0,
		"25":          25,
		"":            0,
		"x/y":         0,
	}
	for in, want := range cases {
		if got := ParseFPS(in); got != want {
			t.Errorf("ParseFPS(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMarkDuplicatesPrefersShortName(t *testing.T) {
	s := []Source{
		{Path: "d/IMG_9812 2.MP4", SHA256: "a"},
		{Path: "d/IMG_9812.MP4", SHA256: "a"},
		{Path: "d/other.MP4", SHA256: "b"},
	}
	markDuplicates(s)
	if s[0].DuplicateOf != "d/IMG_9812.MP4" || s[1].DuplicateOf != "" || s[2].DuplicateOf != "" {
		t.Fatalf("unexpected duplicates: %+v", s)
	}
}
