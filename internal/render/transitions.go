package render

import (
	"fmt"

	"github.com/danmcfan/porcelain/internal/timeline"
)

// xfadeTransitions maps each Transition type to its ffmpeg xfade
// transition: a built-in name, or custom with a per-pixel expression.
var xfadeTransitions = map[string]string{
	timeline.Crossfade: "fade",
	timeline.Flash:     "fadewhite",
	timeline.Dip:       "fadeblack",
	timeline.Zoom:      "zoomin",
	timeline.Pixelize:  "pixelize",
	timeline.Flashbang: "custom:expr='" + flashbangExpr + "'",
	timeline.Glitch:    "custom:expr='" + glitchExpr() + "'",
}

// In xfade's expressions P runs from 1 (all outgoing Clip, A) to 0 (all
// incoming Clip, B), so the cut is at P = 0.5. A, B, X, Y, W and H are the
// current plane's value, position and size; PLANE 0 is luma.

// flashbangExpr holds the outgoing Clip untouched up to the cut, then
// jumps to white (luma 255, neutral chroma 128) and decays into the
// incoming Clip, fast at first.
const flashbangExpr = "if(gt(P,0.5),A,B+(if(PLANE,128,255)-B)*pow(2*P,1.5))"

// glitchExpr splits the frame into 24 horizontal slices. About twelve
// times over the Transition each slice re-rolls whether it shows A or B
// (B more often as P falls) and how far it tears sideways (up to 15% of
// the width, most at the cut); chroma tears a further 4% the other way
// every other step, splitting the color. Every term is inlined: xfade
// evaluates the expression on several threads, which would race on
// st()/ld() variables.
func glitchExpr() string {
	step := "floor((1-P)*12)"
	slice := "floor(Y*24/H)"
	hash := func(a, b, m float64) string {
		return fmt.Sprintf("mod(abs(sin(%s*%g+%s*%g))*%g,1)", slice, a, step, b, m)
	}
	strength := "sin(PI*P)"
	x := fmt.Sprintf("clip(X+(%s-0.5)*W*0.3*%s+if(PLANE,W*0.04*%s*(2*mod(%s,2)-1),0),0,W-1)",
		hash(39.3468, 11.135, 24634.6345), strength, strength, step)
	pick := func(in string) string {
		return fmt.Sprintf("if(eq(PLANE,0),%[1]s0(%[2]s,Y),if(eq(PLANE,1),%[1]s1(%[2]s,Y),%[1]s2(%[2]s,Y)))", in, x)
	}
	return fmt.Sprintf("if(gt(%s,P),%s,%s)", hash(12.9898, 78.233, 43758.5453), pick("b"), pick("a"))
}
