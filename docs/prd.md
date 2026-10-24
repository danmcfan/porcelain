# PRD: AI Band Video Editor (porcelain), Grunge / 90s Aesthetic

> The original PRD was truncated mid-way through FR-1; an agent completed it and the product owner reviewed and confirmed every completed requirement (issue #6). Terms follow `GLOSSARY.md`.

**Product goal:** Turn raw band Footage into a polished, music-synced, fully editable video using AI-powered Shot selection, Creative Prompt-based direction, and a consistent band-specific visual identity.

## 1. Product overview

Build an AI-powered video editing tool that takes a folder of raw Footage, a Song, and a Creative Prompt, then automatically generates a finished music edit.

It should behave like a creative music video editor: it understands the Footage, the Song's rhythm and structure, and the requested aesthetic, then makes intelligent decisions about Shot selection, timing, pacing, Effects, and typography.

The output should feel like an authentic underground 90s grunge / alternative rock music video (raw, gritty, energetic, and intentional) rather than a generic AI-generated template.

**Core principle:** it must create a real, downloadable video with an editable Timeline, not just a montage or a list of suggested clips.

**Delivery:** porcelain is a single `porcelain` binary plus a Skill run from the user's own LLM agent Harness. The Harness does the creative work; the binary does deterministic media work. See ADR 0001.

## 2. Core user workflow

1. Upload a folder of raw band Footage.
2. Upload or select the Song.
3. Enter a Creative Prompt describing the desired edit (style, text, duration, etc.).
4. Generate the edit.
5. Preview the finished video and edit individual elements.
6. Regenerate or create Variations if needed.
7. Export the final video as an editable file (iMovie, Final Cut, CapCut, Instagram Edits, TikTok... however is easiest). See FR-9 for what is feasible.

The workflow should be simple enough that a user can go from raw Footage to a finished edit with minimal manual work.

## 3. Inputs

| Input | Requirements |
| --- | --- |
| Footage | Multiple video files or an entire folder. Support mixed clip lengths, camera angles, and video quality. |
| Song | Upload a song or select an existing track. The Song is the timing and audio source for the edit. |
| Creative Prompt | Free-text direction on mood, pacing, Effects, text, Footage preferences, and creative direction. |
| Brand Kit | The band's actual font files, logo, and optional reference images or videos. |
| Look | A saved default band aesthetic, overridable or refinable through the Creative Prompt. |
| Output settings | Aspect ratio, resolution, and optionally target duration. |

Example Creative Prompt:

> "Create a chaotic, aggressive 90s grunge edit of our live show. Capture the crowd and optimize the edit to have the most energy and biggest feel. Use close-ups of the singer and guitarist, dirty VHS textures, occasional distorted frames, and our band name in our custom font. Make the chorus feel explosive and the intro more atmospheric. Keep the footage raw and authentic."

The Creative Prompt should guide creative decisions, not simply determine which filter is applied.

## 4. Outputs

### A. Finished video

- A complete, rendered, downloadable video file.
- Correctly synced to the Song.
- Includes selected Footage, cuts, transitions, Effects, and typography.
- Uses the requested aspect ratio and export settings.
- Ready to publish on social media or other selected platforms.

### B. Editable project (the Timeline)

- A Timeline containing the actual Clips: their in/out points, ordering, and timing.
- Song and timing information.
- Editable Text Layers, Effects, and transitions.
- Ability to replace, trim, move, delete, or reorder Clips.
- Ability to adjust text, Effects, and timing without rebuilding the entire edit.

### C. Regeneration and Variations

- Ability to generate another edit from the same Footage and Song.
- Ability to change the Creative Prompt and produce a different creative direction.
- Preserve existing Footage and settings when generating Variations unless the user changes them.

### D. Saved Brand Kit

- Store the band's font, logo, color preferences, visual Effects, and default Look.
- Apply these consistently across future edits.

## 5. Functional requirements

### FR-1: Footage analysis and selection

Analyze the Footage and identify useful moments for editing.

- Identify performance shots, close-ups, crowd shots, backstage footage, movement, and visually interesting moments where possible.
- Detect scene changes and avoid selecting only the beginning of each Source.
- Select Footage based on relevance to the Creative Prompt, visual quality, action, variety, and musical context.
- Avoid excessive repetition of the same Shot unless repetition is an intentional stylistic choice.
- Allow users to replace or reject selected Clips.

Acceptance criteria (original, truncated): "Given a folder containing multiple usable clips..." Completed:

- Given a folder of multiple usable Sources, the Timeline draws Clips from most distinct Sources, and Clip in-points are not all at 0s of their Source.
- Every Source is split into Shots and each Shot appears on a Contact Sheet the Harness can view.
- No Shot is used more than once unless the Creative Prompt asks for repetition.
- Duplicate Sources (identical content, different filename) are treated as one Source.
- Rejecting a Clip and regenerating replaces it with a different Shot while leaving other Clips unchanged.

### FR-2: Song analysis and beat sync

- Detect tempo, Beats, and downbeats in the Song.
- Label Sections (intro, verse, chorus, bridge, outro) so the Creative Prompt can target them ("explosive chorus").
- Clip boundaries land on Beats by default; pacing (Clips per bar) follows Section energy.
- Acceptance: in a Render, every Clip boundary is within one frame of a Beat unless the Timeline marks it off-beat on purpose; audio and video stay in sync to within one frame for the whole duration.

### FR-3: Creative Prompt interpretation

- The Harness turns the Creative Prompt into concrete Timeline decisions: Shot choice, pacing per Section, Effects, Look, Text Layers, duration.
- Requests the binary cannot fulfil (e.g. a missing font, a shot type absent from the Footage) are reported back to the user, not silently dropped.
- Acceptance: two clearly different Creative Prompts on the same Footage and Song produce visibly different Timelines. Checked by a person watching both Renders.

### FR-4: Timeline editing

- The Timeline (`timeline.json`) is the single source of truth for every Render and Export (ADR 0003).
- Users (directly or via the Harness) can replace, trim, move, delete, and reorder Clips and adjust Text Layers, Effects, and timing; re-rendering reflects only those changes.
- `porcelain timeline validate` rejects a malformed Timeline with errors that point to the offending field.

### FR-5: Text Layers and Brand Kit

- Text Layers render with the band's own font files (TTF/OTF) and can be timed to Beats or Sections.
- The Brand Kit (band name, fonts, logo, colors, default Look) is saved once and applied to every new Timeline unless the Creative Prompt overrides it.
- Acceptance: a Render with a Brand Kit font shows that font, not a fallback; a missing font is an error.

### FR-6: Effects and Looks

- Provide 90s grunge Effects: VHS texture, film grain, distortion/glitch frames, color grading, speed ramps, flash/strobe on Beats.
- A Look is a named, reusable set of Effects; Effects apply to a Clip or a time range.
- Effects are parameters in the Timeline, never baked into Sources.

### FR-7: Preview

- A fast low-resolution preview Render from Proxies so the user can review before the full Render.

### FR-8: Render

- Render the Timeline to MP4 (H.264 + AAC) at the requested aspect ratio (9:16, 1:1, 16:9, 4:5) and resolution.
- Reframe portrait and landscape Sources to the output aspect ratio (crop or fit, set per Clip).
- Acceptance: the output plays in QuickTime, iOS Photos, and uploads to TikTok/Instagram without re-encoding errors; duration equals the Timeline duration.

### FR-9: Export

- Export the Timeline as FCPXML (Final Cut Pro) and OpenTimelineIO (Resolve, Premiere via adapters).
- iMovie cannot import FCPXML; iMovie, CapCut, TikTok and Instagram Edits users get the rendered MP4.
- Export reports any Effect or Look the target format cannot represent.
- Acceptance: an exported FCPXML and OTIO open in DaVinci Resolve (free, imports both) with Clips at the same in/out points and Timeline positions. Final Cut Pro remains the FCPXML target; Resolve is the check because it needs no paid licence.

### FR-10: Variations

- Generate a Variation as a new Timeline file from the same Footage and Song, with or without a changed Creative Prompt; existing Timelines are never overwritten.
- Variations reuse cached analysis (Proxies, Shots, Beats) so they are fast.

## 6. Non-functional requirements

- **Input formats:** iPhone HEVC `.MOV` (including 4K), H.264 `.MP4`, JPG/HEIC Stills; Songs as MP3, WAV, M4A/AAC. See `docs/footage.md`.
- **Real-footage quirks:** apply rotation flags, handle mixed and variable frame rates, pick the correct audio stream among several, ignore metadata data streams, dedupe identical files.
- **Performance:** analysis runs on Proxies, cached per Source by content hash; originals are decoded only for the final Render. Re-analysing unchanged Footage is near-instant.
- **Distribution:** users build one static `porcelain` binary and the Skill from source (`task skill`); no published releases; no API keys; ffmpeg/ffprobe on `PATH`, checked by `porcelain doctor` (ADRs 0001, 0002).
- **Harness interface:** every command is non-interactive and prints machine-readable JSON.
- **Privacy:** Footage never leaves the user's machine via the binary.
