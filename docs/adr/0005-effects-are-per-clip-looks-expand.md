# 0005: Effects are per Clip with one intensity; Looks expand into Effects

Status: Accepted

## Context

FR-6 asks for six grunge Effects (film grain, color grade, VHS texture, glitch frames, speed ramp, flash/strobe on Beats) and named Looks that group them. We had to decide where Effects attach, what settings they take, and how a Look reaches the Timeline.

## Decision

- An Effect attaches to one Clip. There is no Timeline-level time range. To treat a Section, the Harness puts the Effect on each Clip in it.
- Each Effect takes one setting, `intensity`, from 0 to 1. Validate rejects an unknown Effect type, an unknown param, an intensity outside 0 to 1, and the same Effect type twice on one Clip.
- Looks are presets built into the binary. The Harness expands a Look into Effects on Clips. The Timeline stores only the Effects, never a Look name, so Render needs nothing but the Timeline (ADR 0003). A Clip's own Effects are just Effects; there is no Look-versus-Clip conflict to resolve.
- Speed ramp keeps the Clip's In, Out and Timeline duration unchanged: it remaps time inside the Clip so speed varies but the Clip still plays its full Source range in the same time. Higher intensity means a stronger speed swing. Clips carry no audio, so the Song stays in sync.
- Strobe flashes on the Beats inside the Clip, so it waits for Song analysis.

## Consequences

- Per-Effect settings (e.g. separate saturation and contrast for color grade) can come later as extra params without breaking existing Timelines.
- Looks defined in the Brand Kit are left to the Brand Kit work; the Brand Kit's default Look names a built-in Look.
- Export keeps reporting every Effect it drops.
