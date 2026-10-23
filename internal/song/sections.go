package song

import "math"

// Section is a labeled span of the Song: intro, verse, chorus, bridge or
// outro. Energy is the mean energy of its Bars, from 0 to 1.
type Section struct {
	Label  string  `json:"label"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Energy float64 `json:"energy"`
}

const (
	// contrastDB is the loudness step that starts a new Section and that
	// separates a chorus from quieter Sections.
	contrastDB = 3
	// noveltyBars is how many bars each side of a candidate boundary are
	// compared; it is also the shortest Section.
	noveltyBars = 2
)

// sections splits bars where their loudness steps up or down, then labels
// the spans by energy and order (ADR 0006). The labels are a heuristic; the
// Harness may relabel from the bar energies.
func sections(bars []Bar) []Section {
	db := make([]float64, len(bars))
	for i, b := range bars {
		db[i] = decibels(b.Energy)
	}
	starts := []int{0}
	novelty := make([]float64, len(bars)+1)
	for b := noveltyBars; b <= len(bars)-noveltyBars; b++ {
		novelty[b] = math.Abs(mean(db[b-noveltyBars:b]) - mean(db[b:b+noveltyBars]))
	}
	for b := noveltyBars; b <= len(bars)-noveltyBars; b++ {
		if novelty[b] >= contrastDB && novelty[b] >= novelty[b-1] && novelty[b] > novelty[b+1] &&
			b-starts[len(starts)-1] >= noveltyBars {
			starts = append(starts, b)
		}
	}
	out := make([]Section, len(starts))
	for i, s := range starts {
		end := len(bars)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		var e float64
		for _, b := range bars[s:end] {
			e += b.Energy
		}
		out[i] = Section{Start: bars[s].Start, End: bars[end-1].End, Energy: round(e / float64(end-s))}
	}
	label(out)
	return out
}

// label names Sections: the loudest are choruses; a distinctly quiet first
// or last Section is the intro or outro; a quieter Section after the second
// chorus that leads into another is the bridge; the rest are verses.
// Without loudness contrast every Section is a verse.
func label(secs []Section) {
	loudest := math.Inf(-1)
	for _, s := range secs {
		loudest = max(loudest, decibels(s.Energy))
	}
	loud := func(i int) bool { return decibels(secs[i].Energy) >= loudest-contrastDB }
	var quiet []int
	for i := range secs {
		if !loud(i) {
			quiet = append(quiet, i)
		}
	}
	if len(quiet) == 0 {
		for i := range secs {
			secs[i].Label = "verse"
		}
		return
	}
	last := len(secs) - 1
	// edge reports whether quiet Section i is quieter than every other quiet
	// Section in the Song's body, so it reads as an intro or outro.
	edge := func(i int) bool {
		for _, j := range quiet {
			if j != 0 && j != last && decibels(secs[i].Energy) > decibels(secs[j].Energy)-contrastDB {
				return false
			}
		}
		return true
	}
	choruses := 0
	for i := range secs {
		switch {
		case loud(i):
			secs[i].Label = "chorus"
			choruses++
		case i == 0 && last > 0 && edge(i):
			secs[i].Label = "intro"
		case i == last && last > 0 && edge(i):
			secs[i].Label = "outro"
		case choruses >= 2 && i < last && loud(i+1):
			secs[i].Label = "bridge"
		default:
			secs[i].Label = "verse"
		}
	}
}

// decibels converts a 0-1 energy to dB, flooring silence at -80 dB.
func decibels(e float64) float64 { return 20 * math.Log10(max(e, 1e-4)) }

func mean(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}
