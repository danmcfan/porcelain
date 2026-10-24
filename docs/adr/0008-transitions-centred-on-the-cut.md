# 0008: Transitions are set on the incoming Clip and centred on the cut

Status: Accepted

## Context

Hard cuts were the only way from one Clip to the next. Users want real Transitions (dissolves, flashes, zooms). A Transition overlaps two Clips, which conflicts with the rule that Clips are contiguous and each starts on its Beat.

## Decision

- A Clip may carry one `transition` (`type`, `duration`) that blends the previous Clip into it. The first Clip cannot have one.
- The Transition is centred on the cut. Render plays `duration/2` of Source past the previous Clip's Out and before this Clip's In, so every Clip keeps its `at`, the Timeline keeps its length and the midpoint of the Transition stays on the Beat the Harness cut on.
- Validate rejects a Transition that needs Source the file does not have, or that is longer than either Clip.
- Types map to ffmpeg `xfade` transitions inside the render package. Each Clip's Effects apply to its stretched range before the blend.

## Consequences

- Stills can always be stretched; video Clips need a little Source either side of the cut.
- Export does not carry Transitions yet; it reports each one as dropped.
