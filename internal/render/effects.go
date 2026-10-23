package render

import (
	"fmt"
	"math"
	"strings"

	"github.com/danmcfan/porcelain/internal/timeline"
)

// resampleFilter returns the filter, followed by a comma, that resamples c
// to fps. With a speed_ramp it first remaps time inside the Clip: an input
// frame at T plays at T − a·d/2π·sin(2πT/d), so the Clip whips in fast,
// slows into the middle and whips out. d is the time of the Clip's last
// output frame, so the first and last frames stay on In and Out and the
// Clip keeps its duration; a < 1 keeps time moving forward. framerate
// blends neighbouring frames, so the uneven steps don't judder and slow
// motion on 30 fps Footage doesn't show frames twice.
func resampleFilter(c timeline.Clip, fps int) string {
	for _, e := range c.Effects {
		if e.Type == timeline.SpeedRamp && e.Intensity() > 0 {
			a, d := 0.8*e.Intensity(), c.Duration()-1/float64(fps)
			return fmt.Sprintf("setpts=PTS-STARTPTS,setpts='(T-%.4f*sin(2*PI*T/%.4f))/TB',framerate=fps=%d,", a*d/(2*math.Pi), d, fps)
		}
	}
	return fmt.Sprintf("fps=%d,", fps)
}

// effectFilters returns the filters, each followed by a comma, for c's
// Effects other than speed_ramp, in timeline.EffectTypes order. beats are
// in Clip time (0 at the Clip's first frame). i is the Clip's index, which
// keeps glitch's stream labels unique in the graph. An Effect at intensity
// 0 adds nothing.
func effectFilters(c timeline.Clip, i int, beats []float64, fps int) string {
	var b strings.Builder
	for _, typ := range timeline.EffectTypes {
		for _, e := range c.Effects {
			if e.Type != typ || e.Intensity() == 0 {
				continue
			}
			k := e.Intensity()
			switch typ {
			case timeline.ColorGrade:
				// Faded blacks, an S-curve, teal-green shadows, warm
				// highlights and less saturation.
				fmt.Fprintf(&b, "curves=m='0/%.3f 0.25/%.3f 0.75/%.3f 1/%.3f',colorbalance=rs=%.3f:gs=%.3f:bs=%.3f:rm=%.3f:gm=%.3f:rh=%.3f:bh=%.3f,eq=saturation=%.3f,",
					0.12*k, 0.25-0.06*k, 0.75+0.08*k, 1-0.1*k,
					-0.2*k, 0.06*k, 0.18*k, -0.04*k, 0.08*k, 0.1*k, -0.12*k, 1-0.55*k)
			case timeline.VHS:
				// Horizontal luma softness, chroma smeared and shifted
				// sideways, lifted blacks, tape noise and scanlines.
				s := math.Round(8 * k)
				fmt.Fprintf(&b, "gblur=sigma=%.3f:sigmaV=%.3f:planes=1,gblur=sigma=%.3f:sigmaV=%.3f:planes=6,chromashift=cbh=%.0f:crh=%.0f,"+
					"eq=contrast=%.3f:brightness=%.3f:saturation=%.3f,noise=c0s=%.0f:c0f=t:c0_seed=7,drawgrid=w=iw:h=6:t=2:c=black@%.3f,",
					2.5*k, 0.4*k, 12*k, k, s, -s, 1-0.15*k, 0.04*k, 1+0.15*k, math.Round(10*k), 0.3*k)
			case timeline.FilmGrain:
				// Temporal grain, seeded so Renders repeat.
				fmt.Fprintf(&b, "noise=alls=%.0f:allf=t:all_seed=7,", math.Round(35*k))
			case timeline.Glitch:
				// Bursts of three frames, picked by frame number, tear two
				// slices sideways and split the color, flipping direction
				// each frame; one frame in three also jumps hue. Higher
				// intensity means more bursts, torn further.
				burst := fmt.Sprintf("lt(mod(floor(n/3)*7,19),%.3f)", 5*k)
				flip := "(2*mod(n,2)-1)"
				y1, y2 := "0.85*mod(floor(n/3)*0.37,1)", "0.85*mod(floor(n/3)*0.61+0.3,1)"
				s := math.Round(40 * k)
				fmt.Fprintf(&b, "split=3[g%[1]dm][g%[1]da][g%[1]db];"+
					"[g%[1]da]crop=iw:ih*0.10:0:'ih*%[2]s'[g%[1]dc];[g%[1]db]crop=iw:ih*0.04:0:'ih*%[3]s'[g%[1]dd];"+
					"[g%[1]dm][g%[1]dc]overlay=x='W*%.3[4]f*%[5]s':y='H*%[2]s':enable='%[6]s'[g%[1]de];"+
					"[g%[1]de][g%[1]dd]overlay=x='-W*%.3[7]f*%[5]s':y='H*%[3]s':enable='%[6]s',"+
					"chromashift=cbh=%.0[8]f:crh=%.0[9]f:cbv=%.0[10]f:enable='%[6]s*mod(n,2)',"+
					"chromashift=cbh=%.0[11]f:crh=%.0[8]f:crv=%.0[11]f:enable='%[6]s*(1-mod(n,2))',"+
					"hue=H=%.3[12]f:enable='%[6]s*eq(mod(n,3),1)',",
					i, y1, y2, 0.08*k, flip, burst, 0.16*k, s, -s, s/2, -s/2, 0.6*math.Pi*k)
			case timeline.Strobe:
				// An overexposed flash on each Beat: luma lifts and color
				// washes out, then fades within a frame or so.
				var on []string
				half := 0.5 / float64(fps)
				for _, t := range beats {
					if t > -half && t < c.Duration() {
						on = append(on, fmt.Sprintf("between(t,%.4f,%.4f)*exp(-max(t-%.4f,0)/0.03)", t-half, t+0.2, t))
					}
				}
				if len(on) > 0 {
					f := strings.Join(on, "+")
					fmt.Fprintf(&b, "eq=brightness='%.3f*(%s)':saturation='1-%.3f*(%s)':eval=frame,", 0.75*k, f, 0.6*k, f)
				}
			}
		}
	}
	return b.String()
}

// hasEffect reports whether any Clip in t has an Effect of type typ above
// intensity 0.
func hasEffect(t *timeline.Timeline, typ string) bool {
	for _, c := range t.Clips {
		for _, e := range c.Effects {
			if e.Type == typ && e.Intensity() > 0 {
				return true
			}
		}
	}
	return false
}
