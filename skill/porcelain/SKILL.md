---
name: porcelain
description: Edits a band's phone Footage into a music video cut to their Song, using the `porcelain` CLI. Use when the user wants a video, reel, or Variation made from a folder of clips and photos, or wants an existing porcelain Timeline changed.
---

# porcelain

You are the creative half; `porcelain` is the deterministic half. You read the Creative Prompt and Brand Kit, choose moments from the Footage, and write a Timeline (`timeline.json`). The binary probes, validates, and renders. The Timeline is the single source of truth: every change is a Timeline edit followed by a fresh Render, never an edit to the video file.

## Locating the binary

Use `porcelain` if it is on `PATH`; otherwise `bin/porcelain` (`bin/porcelain.exe` on Windows) in the folder holding this file. Every command prints JSON to stdout. On failure it prints `{"error", "code", "details"}` to stderr and exits non-zero; exit code 2 with `"code": "not_implemented"` means the feature does not exist yet, so work around it rather than retry.

## Steps

1. **Doctor.** Run `porcelain doctor`. Done when it prints `"ok": true`. Otherwise tell the user to install ffmpeg (it must provide `ffmpeg` and `ffprobe` on `PATH`) and stop.
2. **Probe.** Run `porcelain probe <footage-folder>`. Done when you hold one record per Source with its `kind`, `width`x`height` (already upright: rotation applied), `fps`, and `duration`. Drop every Source with `duplicate_of` set, and every `kind: "other"`. Note portrait vs landscape: an output aspect that differs from a Source's crops it.
3. **See the Footage.** Run `porcelain shots <footage-folder>`: it prints `shots` (`source`, `start`, `end` in Source seconds) and `skipped` (duplicates, Stills, unreadable files, each with a `reason`). Then run `porcelain contact-sheet <footage-folder>` and open every `image` it lists: each image has `columns` frames per row; every Shot starts a new row and long Shots wrap onto more rows (about one frame every 2.5 s), possibly continuing on the next image, so use `cells` (row-major), which gives each frame's `shot` index and `time`. Look at Stills directly. Done when you know what is in each Shot (performance, close-up, crowd, backstage, movement) and can pick Clip ranges inside Shots rather than at the start of each Source. The first run on new Footage transcodes Proxies and can take a while; later runs reuse the cache.
4. **Song.** Run `porcelain analyze-song <song>`. It prints `duration`, `tempo` (BPM), `beats` and `downbeats` (Song seconds), `bars` (`start`, `end`, `energy` from 0 to 1, loudest Bar = 1) and `sections` (`label`, `start`, `end`, `energy`). Done when you know where each Section is and how hard it hits. Put Clip boundaries on `beats`; start big changes on `downbeats`; use more Clips per Bar in high-energy Sections. The Section labels come from loudness alone (the loudest Sections are `chorus`; a quiet first or last Section is `intro` or `outro`), so if they contradict what you hear in the `bars` energy curve or what the user tells you about the Song, trust that and relabel. When you use `song.start`, subtract it from these times to get Timeline times. Four Beats per bar is assumed: for a Song in 3/4 or 6/8, ignore `downbeats` and `bars` and work from `beats`.
5. **Brand Kit.** Run `porcelain brand-kit show`. If it fails with `"code": "no_brand_kit"`, ask the user for the band name, font files (TTF/OTF), logo, colors and default Look (a `name` from `porcelain looks`), write them to a JSON file (`band_name`, `fonts`, `logo`, `colors` as `#RRGGBB`, `default_look`; paths relative to that file), and run `porcelain brand-kit save <file>`; fix any `details` it reports. Done when you hold the Brand Kit. Apply it to every new Timeline (Text Layers in its first font and its colors, its default Look) unless the Creative Prompt says otherwise.
6. **Write the Timeline.** Write `timeline.json` following `timeline.schema.json` beside this file. Done when it expresses the Creative Prompt and Brand Kit and every Clip obeys the rules below.
7. **Validate.** Run `porcelain timeline validate timeline.json`. Done when it prints `"valid": true`. Each entry in `details` names a `field` and a `message`: fix exactly those and re-run.
8. **Render.** Run `porcelain render timeline.json -o render.mp4`. Done when it prints the output path and duration.
9. **Export (if asked).** When the user wants to keep editing in an NLE, run `porcelain export timeline.json --format fcpxml -o timeline.fcpxml` for Final Cut Pro, or `--format otio -o timeline.otio` for DaVinci Resolve, Premiere (via an OTIO adapter) and others. iMovie, CapCut, TikTok and Instagram Edits cannot import either: give those users the Render. Done when it prints the output path. Tell the user about every entry in `dropped` (Effects and Text Layers the format cannot carry) so they can redo it in the editor. The export links to the original Footage by absolute path, so it must stay where it is.
10. **Show the user** the Render path and a one-paragraph summary of your choices (which moments, why, how the cuts follow the Song).
11. **Iterate.** For a change, edit the same `timeline.json` and repeat steps 7 to 10. For a Variation (a different take on the same Footage), write a new Timeline file such as `timeline-v2.json` and render it to its own output, keeping the earlier one intact.

## Timeline rules

- Paths are relative to the `timeline.json` file (absolute paths also work).
- `output.aspect` is `9:16` (1080x1920), `1:1` (1080x1080), or `16:9` (1920x1080); `output.fps` is one of 24, 25, 30, 50, 60. Sources are scaled to cover the frame and center-cropped.
- Clips play in array order, back to back: the first has `at: 0`, each next one has `at` = previous `at` + (`out` - `in`).
- `out` stays within the Source's `duration`. For a Still, `in` is 0 and `out` is how long it is shown.
- The Song is the only audio; Source audio is dropped. `song.start` skips into the Song. The Timeline may be shorter than the Song (the Song is cut off) but never longer.
- `text_layers` are drawn over the Clips while `start` <= t < `end` (Timeline seconds). Time them to `beats` or `sections`. `font` is a font file path, normally a Brand Kit font; a missing font fails validation, so never invent one. `size` is the text height in output pixels (default 1/12 of the frame height), `color` is `#RRGGBB` (default white), `position` is `top`, `center` (default) or `bottom`. Use `\n` in `text` for multiple lines; long lines are not wrapped, so keep them short enough for the frame.
- `effects` on a Clip are visual treatments, each `{"type": ..., "params": {"intensity": 0 to 1}}`. Intensity 0 looks like no Effect; 1 is strongest. Each type at most once per Clip; no other params. Types, in the order Render applies them:
  - `speed_ramp`: the Clip whips in fast, slows into the middle, and whips out. The Clip still plays its full `in` to `out` range in the same time, so cuts stay on the Beats.
  - `color_grade`: 90s film grade: faded blacks, contrasty mids, teal shadows, warm highlights, less saturation.
  - `vhs`: soft picture, color smeared sideways, lifted blacks, tape noise and scanlines.
  - `film_grain`: moving grain.
  - `glitch`: short bursts of torn slices, color splits and hue jumps on scattered frames.
  - `strobe`: an overexposed flash on every Song Beat inside the Clip, fading within a couple of frames. Render finds the Beats itself; you need not list them.
- `transition` on a Clip (not the first) blends the previous Clip into it: `{"type": ..., "duration": seconds}`. Types: `crossfade` (dissolve), `flash` (fades up through white and back down), `flashbang` (the outgoing Clip plays untouched to the cut, then a hard white hit that decays into the next Clip; the punchiest), `dip` (through black), `zoom` (punches into the outgoing Clip), `pixelize` (blocks resolve into the next Clip), `glitch` (the frame tears into flickering sideways slices of both Clips with a color split; pairs well with the `vhs` and `glitch` Effects). It is centred on the cut, so keep cutting on Beats; `at` values do not change. Render plays `duration/2` of extra Source after the previous Clip's `out` and before this Clip's `in`, so leave that much room in both Sources. Keep it short (a Beat or two) in fast edits; a hard cut (no `transition`) is still the default.
- `reframe` on a Clip zooms into its Source to focus on what matters, e.g. the performers or crowd in a wide shot: `{"zoom": 1 to 4, "x": 0 to 1, "y": 0 to 1}`. `zoom` is magnification over the default cover crop (1 changes nothing but the focus point); `x`/`y` is the focus point in the upright Source (0,0 top left), and the view is centred on it but never runs past the Source's edges. Find the focus point on the Contact Sheet. Add `zoom_end` for a slow push-in (larger than `zoom`) or pull-out (smaller) across the Clip. Each step of zoom upscales: past about 2 on 1080p Sources the picture goes soft.
- **Looks.** Run `porcelain looks` for the built-in Looks (`name` and `effects`). A Timeline never names a Look: to apply one, copy its `effects` onto each Clip it should cover. To treat a Section, put the Effects on every Clip that overlaps that Section's time range. Use the Brand Kit's `default_look` unless the Creative Prompt asks otherwise, and adjust intensities to taste (e.g. strobe only on `chorus` Clips). If a Clip already has an Effect of the same type, keep one.

## Example

```json
{
  "schema_version": 1,
  "output": { "aspect": "9:16", "fps": 30 },
  "song": { "path": "song.wav" },
  "clips": [
    { "source": "footage/IMG_5465.MOV", "in": 0.5, "out": 2.5, "at": 0 },
    { "source": "footage/IMG_5466.JPG", "in": 0, "out": 1, "at": 2 },
    { "source": "footage/IMG_5468.MOV", "in": 3, "out": 5, "at": 3 }
  ],
  "notes": "Opens on the crowd shot to match the Creative Prompt's 'start big'."
}
```
