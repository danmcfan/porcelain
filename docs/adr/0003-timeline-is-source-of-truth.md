# 0003: The Timeline file is the single source of truth

Status: Accepted

## Context

The PRD requires a real editable project, not just a rendered video, plus Variations and export to other editors. If Render, Export and edits each held their own state, they would drift.

## Decision

`timeline.json` is the single source of truth. Render and every Export derive from it. A Variation is a new Timeline file. Editing the edit means editing the Timeline (by the Harness or a human) and re-rendering.

Export targets:
- FCPXML for Final Cut Pro. iMovie cannot import FCPXML; iMovie users get the rendered MP4.
- OpenTimelineIO (OTIO) for DaVinci Resolve, Premiere (via adapters) and other NLEs.
- CapCut, TikTok and Instagram Edits get the rendered MP4; they have no importable project format we can target.

## Consequences

- The Timeline schema is a public contract: version it and validate it (`porcelain timeline validate`) before Render or Export.
- Exports are lossy where the target cannot express a porcelain Effect or Look; Export must report what it dropped.
- Render must be reproducible from the Timeline plus Footage alone.
