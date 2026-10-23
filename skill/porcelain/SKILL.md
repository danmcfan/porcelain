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
4. **Song.** *Not implemented yet:* `porcelain analyze-song <song>` (Beats and Sections). Until then, get the Song's duration from `porcelain probe <song>` and, if the user knows the BPM, place cuts on multiples of `60 / BPM` seconds.
5. **Write the Timeline.** Write `timeline.json` following `timeline.schema.json` beside this file. Done when it expresses the Creative Prompt and Brand Kit and every Clip obeys the rules below.
6. **Validate.** Run `porcelain timeline validate timeline.json`. Done when it prints `"valid": true`. Each entry in `details` names a `field` and a `message`: fix exactly those and re-run.
7. **Render.** Run `porcelain render timeline.json -o render.mp4`. Done when it prints the output path and duration.
8. **Show the user** the Render path and a one-paragraph summary of your choices (which moments, why, how the cuts follow the Song).
9. **Iterate.** For a change, edit the same `timeline.json` and repeat steps 6 to 8. For a Variation (a different take on the same Footage), write a new Timeline file such as `timeline-v2.json` and render it to its own output, keeping the earlier one intact.

## Timeline rules

- Paths are relative to the `timeline.json` file (absolute paths also work).
- `output.aspect` is `9:16` (1080x1920), `1:1` (1080x1080), or `16:9` (1920x1080); `output.fps` is one of 24, 25, 30, 50, 60. Sources are scaled to cover the frame and center-cropped.
- Clips play in array order, back to back: the first has `at: 0`, each next one has `at` = previous `at` + (`out` - `in`).
- `out` stays within the Source's `duration`. For a Still, `in` is 0 and `out` is how long it is shown.
- The Song is the only audio; Source audio is dropped. `song.start` skips into the Song. The Timeline may be shorter than the Song (the Song is cut off) but never longer.
- *Not rendered yet:* `effects` on Clips and `text_layers`. They validate, but render refuses them with `not_implemented`; leave them out of anything you render.
- *Not implemented yet:* `porcelain export` (platform-specific delivery). Hand the user the Render instead.

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
