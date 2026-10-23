package timeline

// Look is a named set of Effects built into porcelain. The Harness copies a
// Look's Effects onto Clips; a Timeline never names a Look (ADR 0005).
type Look struct {
	Name    string   `json:"name"`
	Effects []Effect `json:"effects"`
}

// Looks are the built-in Looks.
var Looks = []Look{
	{Name: "VHS", Effects: []Effect{NewEffect(VHS, 0.8), NewEffect(ColorGrade, 0.4), NewEffect(FilmGrain, 0.3)}},
	{Name: "Grain", Effects: []Effect{NewEffect(FilmGrain, 0.7), NewEffect(ColorGrade, 0.6)}},
	{Name: "Distortion", Effects: []Effect{NewEffect(Glitch, 0.6), NewEffect(VHS, 0.5), NewEffect(ColorGrade, 0.3)}},
	{Name: "Strobe", Effects: []Effect{NewEffect(Strobe, 0.6), NewEffect(ColorGrade, 0.5)}},
}
