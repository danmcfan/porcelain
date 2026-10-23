package render

import (
	"fmt"
	"math"
	"strings"

	"github.com/danmcfan/porcelain/internal/timeline"
)

// rampFilter remaps time inside a Clip of length dur for a speed_ramp: an
// input frame at T plays at T + a·dur/2π·sin(2πT/dur). Both ends stay put,
// so the Clip still runs from In to Out in dur; a < 1 keeps time moving
// forward. It returns "" when c has no speed_ramp.
func rampFilter(c timeline.Clip) string {
	for _, e := range c.Effects {
		if e.Type == timeline.SpeedRamp && e.Intensity() > 0 {
			a, d := 0.8*e.Intensity(), c.Duration()
			return fmt.Sprintf("setpts=PTS-STARTPTS,setpts='(T+%.4f*sin(2*PI*T/%.4f))/TB',", a*d/(2*math.Pi), d)
		}
	}
	return ""
}

// effectFilters returns the filters, each followed by a comma, for c's
// Effects other than speed_ramp, in timeline.EffectTypes order. beats are
// in Clip time (0 at the Clip's first frame). An Effect at intensity 0 adds
// nothing.
func effectFilters(c timeline.Clip, beats []float64, fps int) string {
	var b strings.Builder
	for _, typ := range timeline.EffectTypes {
		for _, e := range c.Effects {
			if e.Type != typ || e.Intensity() == 0 {
				continue
			}
			k := e.Intensity()
			switch typ {
			case timeline.ColorGrade:
				// Washed-out, contrasty, green-teal shadows.
				fmt.Fprintf(&b, "eq=contrast=%.3f:saturation=%.3f:gamma=%.3f,colorbalance=rs=%.3f:gs=%.3f:bs=%.3f:rh=%.3f:bh=%.3f,",
					1+0.3*k, 1-0.5*k, 1-0.1*k, -0.1*k, 0.05*k, 0.08*k, 0.08*k, -0.06*k)
			case timeline.VHS:
				// Chroma bleed, softness and scanlines.
				s := math.Round(6 * k)
				fmt.Fprintf(&b, "chromashift=cbh=%.0f:crh=%.0f,gblur=sigma=%.3f,eq=saturation=%.3f,drawgrid=w=iw:h=4:t=1:c=black@%.3f,",
					s, -s, 1.5*k, 1-0.3*k, 0.4*k)
			case timeline.FilmGrain:
				// Temporal grain (seeded, so Renders repeat) and a dark vignette.
				fmt.Fprintf(&b, "noise=alls=%.0f:allf=t:all_seed=7,vignette=angle=%.4f,", math.Round(35*k), math.Pi/3*k)
			case timeline.Glitch:
				// Fixed pseudo-random frames get a hard chroma split and hue
				// jump; higher intensity hits more frames, harder.
				on := fmt.Sprintf("enable='lt(mod(n*7,19),%.3f)'", 5*k)
				s := math.Round(40 * k)
				fmt.Fprintf(&b, "chromashift=cbh=%.0f:crh=%.0f:cbv=%.0f:%s,hue=H=%.3f:%s,", s, -s, s/2, on, math.Pi*k, on)
			case timeline.Strobe:
				// A one-frame white flash on each Beat.
				var on []string
				half := 0.5 / float64(fps)
				for _, t := range beats {
					if t > -half && t < c.Duration() {
						on = append(on, fmt.Sprintf("between(t,%.4f,%.4f)", t-half, t+half))
					}
				}
				if len(on) > 0 {
					fmt.Fprintf(&b, "drawbox=x=0:y=0:w=iw:h=ih:t=fill:color=white@%.3f:enable='%s',", k, strings.Join(on, "+"))
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
