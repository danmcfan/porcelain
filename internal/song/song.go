// Package song analyses the Song: tempo, Beats, Downbeats, a per-Bar energy
// curve and labeled Sections (ADR 0006). Everything is deterministic: the
// same Song always gives the same Analysis.
package song

import (
	"context"
	"fmt"
	"math"
	"math/cmplx"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
)

const (
	rate = 24000 // decode sample rate
	hop  = 240   // onset frame step: 10 ms
	win  = 512   // FFT window: short, so onsets are not smeared early
	fps  = float64(rate) / hop

	beatsPerBar = 4 // porcelain assumes 4/4
)

// Analysis is the result of `porcelain analyze-song`.
type Analysis struct {
	Song      string    `json:"song"`
	Duration  float64   `json:"duration"`
	Tempo     float64   `json:"tempo"`
	Beats     []float64 `json:"beats"`
	Downbeats []float64 `json:"downbeats"`
	Bars      []Bar     `json:"bars"`
	Sections  []Section `json:"sections"`
}

// Bar is one Bar, from a Downbeat to the next. Energy is its loudness
// relative to the Song's loudest Bar, from 0 to 1.
type Bar struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Energy float64 `json:"energy"`
}

// Analyze decodes the Song at path and analyses it.
func Analyze(ctx context.Context, path string) (*Analysis, error) {
	samples, err := ffmpeg.DecodeAudio(ctx, path, rate)
	if err != nil {
		return nil, err
	}
	if len(samples) < rate {
		return nil, fmt.Errorf("%s: Song is shorter than one second", path)
	}
	a := analyzePCM(samples)
	a.Song = path
	return a, nil
}

// analyzePCM analyses mono samples at rate.
func analyzePCM(samples []float32) *Analysis {
	onset := onsetStrength(samples)
	period := tempoPeriod(onset)
	frames := trackBeats(onset, period)
	a := &Analysis{
		Duration: round(float64(len(samples)) / rate),
		Tempo:    round(60 * fps / period),
		Beats:    make([]float64, len(frames)),
	}
	for i, f := range frames {
		a.Beats[i] = round(float64(f) / fps)
	}
	a.Downbeats = downbeats(a.Beats, frames, onset)
	a.Bars = bars(samples, a.Downbeats, a.Duration)
	a.Sections = sections(a.Bars)
	return a
}

// onsetStrength is the spectral flux of log magnitudes, one value per hop,
// normalised to unit standard deviation. Frame t is centred on sample t*hop.
func onsetStrength(samples []float32) []float64 {
	n := len(samples)/hop + 1
	window := make([]float64, win)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/win)
	}
	buf := make([]complex128, win)
	prev := make([]float64, win/2)
	cur := make([]float64, win/2)
	onset := make([]float64, n)
	for t := range n {
		start := t*hop - win/2
		for i := range buf {
			var s float64
			if j := start + i; j >= 0 && j < len(samples) {
				s = float64(samples[j])
			}
			buf[i] = complex(s*window[i], 0)
		}
		fft(buf)
		var flux float64
		for k := range cur {
			cur[k] = math.Log1p(100 * cmplx.Abs(buf[k]))
			if d := cur[k] - prev[k]; d > 0 {
				flux += d
			}
		}
		// A window running past the end sees the Song cut off, not an onset.
		if start+win <= len(samples) {
			onset[t] = flux
		}
		prev, cur = cur, prev
	}
	var sum, sq float64
	for _, v := range onset {
		sum += v
		sq += v * v
	}
	mean := sum / float64(n)
	if sd := math.Sqrt(sq/float64(n) - mean*mean); sd > 0 {
		for i := range onset {
			onset[i] /= sd
		}
	}
	return onset
}

// fft is an in-place radix-2 FFT; len(x) must be a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], wk*x[start+k+size/2]
				x[start+k], x[start+k+size/2] = a+b, a-b
				wk *= w
			}
		}
	}
}

// tempoPeriod is the beat period, in onset frames, from the autocorrelation
// of the onset strength, weighted towards 120 BPM so a tempo is not mistaken
// for its half or double.
func tempoPeriod(onset []float64) float64 {
	var mean float64
	for _, v := range onset {
		mean += v
	}
	mean /= float64(len(onset))
	minLag, maxLag := int(60*fps/200), int(60*fps/40)
	maxLag = min(maxLag, len(onset)/2)
	acf := make([]float64, maxLag+2)
	for lag := minLag - 1; lag <= maxLag+1 && lag < len(onset); lag++ {
		var s float64
		for t := lag; t < len(onset); t++ {
			s += (onset[t] - mean) * (onset[t-lag] - mean)
		}
		acf[min(lag, len(acf)-1)] = s / float64(len(onset)-lag)
	}
	best, bestScore := minLag, math.Inf(-1)
	for lag := minLag; lag <= maxLag; lag++ {
		octaves := math.Log2(60 * fps / float64(lag) / 120)
		if score := acf[lag] * math.Exp(-0.5*octaves*octaves); score > bestScore {
			best, bestScore = lag, score
		}
	}
	// Parabolic interpolation between neighbouring lags.
	l, c, r := acf[best-1], acf[best], acf[min(best+1, len(acf)-1)]
	if d := l - 2*c + r; d < 0 {
		return float64(best) + 0.5*(l-r)/d
	}
	return float64(best)
}

// trackBeats picks beat frames by dynamic programming (Ellis 2007): each beat
// rewards onset strength and penalises spacing away from period.
func trackBeats(onset []float64, period float64) []int {
	const tightness = 100
	n := len(onset)
	score := make([]float64, n)
	back := make([]int, n)
	for t := range n {
		back[t] = -1
		best := math.Inf(-1)
		for p := t - int(math.Round(2*period)); p <= t-int(math.Round(period/2)); p++ {
			if p < 0 {
				continue
			}
			l := math.Log(float64(t-p) / period)
			if s := score[p] - tightness*l*l; s > best {
				best, back[t] = s, p
			}
		}
		score[t] = onset[t]
		if back[t] >= 0 && best > 0 {
			score[t] += best
		} else {
			back[t] = -1
		}
	}
	last := 0
	for t := range n {
		if score[t] > score[last] {
			last = t
		}
	}
	var frames []int
	for t := last; t >= 0; t = back[t] {
		frames = append(frames, t)
	}
	for i, j := 0, len(frames)-1; i < j; i, j = i+1, j-1 {
		frames[i], frames[j] = frames[j], frames[i]
	}
	return trimWeak(frames, onset)
}

// trimWeak drops Beats at either end that fall on near-silence, which the
// beat tracker adds when the Song starts or ends without a beat.
func trimWeak(frames []int, onset []float64) []int {
	var mean float64
	for _, f := range frames {
		mean += onset[f]
	}
	mean /= float64(len(frames))
	for len(frames) > 1 && onset[frames[len(frames)-1]] < 0.1*mean {
		frames = frames[:len(frames)-1]
	}
	for len(frames) > 1 && onset[frames[0]] < 0.1*mean {
		frames = frames[1:]
	}
	return frames
}

// downbeats returns every fourth Beat, starting at the phase with the
// strongest onsets. Without a clear accent the bar starts on the first Beat.
func downbeats(beats []float64, frames []int, onset []float64) []float64 {
	var accent [beatsPerBar]float64
	var count [beatsPerBar]int
	for i, f := range frames {
		accent[i%beatsPerBar] += onset[f]
		count[i%beatsPerBar]++
	}
	phase := 0
	for p := range beatsPerBar {
		if count[p] > 0 {
			accent[p] /= float64(count[p])
		}
		if accent[p] > 1.1*accent[phase] {
			phase = p
		}
	}
	var out []float64
	for i := phase; i < len(beats); i += beatsPerBar {
		out = append(out, beats[i])
	}
	return out
}

// bars splits the Song at each downbeat; the first bar absorbs any pickup
// before the first downbeat and the last runs to the end of the Song.
func bars(samples []float32, downbeats []float64, duration float64) []Bar {
	var starts []float64
	for i, d := range downbeats {
		if i == 0 || d > 0 {
			starts = append(starts, d)
		}
	}
	if len(starts) == 0 {
		starts = []float64{0}
	}
	starts[0] = 0
	out := make([]Bar, len(starts))
	var loudest float64
	for i, s := range starts {
		end := duration
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		lo, hi := int(s*rate), min(int(end*rate), len(samples))
		var sq float64
		for _, v := range samples[lo:hi] {
			sq += float64(v) * float64(v)
		}
		rms := math.Sqrt(sq / float64(max(hi-lo, 1)))
		out[i] = Bar{Start: s, End: end, Energy: rms}
		loudest = max(loudest, rms)
	}
	for i := range out {
		if loudest > 0 {
			out[i].Energy = round(out[i].Energy / loudest)
		}
	}
	return out
}

// round rounds to the millisecond, so the JSON stays readable.
func round(v float64) float64 { return math.Round(v*1000) / 1000 }
