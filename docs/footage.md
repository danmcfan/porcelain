# Real footage quirks

What real phone Footage looks like, and what each quirk demands of the code. Surveyed from the "ALS Fundraiser Sample" with ffprobe. The sample is not in git (ADR 0004): point `PORCELAIN_FOOTAGE_DIR` at a local copy. Each quirk has a matching Fixture in `scripts/gen-fixtures.sh`, except the Apple metadata data streams, which ffmpeg cannot synthesise.

## The sample

| Kind | Count | Format |
| --- | --- | --- |
| iPhone `.MOV` | 29 | HEVC 3840x2160, 1.2-25s each (~3m23s total) |
| `.MP4` | 3 unique (6 files) | H.264 720x1280, 30fps |
| `.JPG` Stills | 10 | 5712x4284 (one 4032x3024) |
| Song | none | the user must supply one |

## Quirks and what the code must do

- **Rotation side data.** 27 of 29 MOVs are stored 3840x2160 with a `-90` display-matrix rotation, so they display portrait; 2 are true landscape. Read rotation from side data and apply it in Proxies, Contact Sheets and Render. Report the displayed (rotated) width/height, never the coded ones.
- **HEVC.** Decoding 4K HEVC is slow. Do analysis on Proxies, never on originals; touch originals only in the final Render.
- **Mixed frame rates.** Mostly 60fps, three 30fps, one 59.94. Proxies are constant-frame-rate; Render normalises to the Timeline's output fps. Express Timeline times in seconds, not frames of a Source.
- **Two audio streams per MOV.** One `aac`, one unknown codec (Apple APAC spatial audio) that ffmpeg may not decode. Select the first `aac` stream explicitly; never rely on ffmpeg's default stream choice.
- **Apple data streams.** Each MOV carries 3-5 metadata data streams. Ignore them: map only the chosen video and audio streams, or ffmpeg may fail or copy junk into outputs.
- **Exact duplicates.** Each MP4 appears twice (`IMG_xxxx.MP4` and `IMG_xxxx 2.MP4`). Dedupe Sources by content hash, not filename, so the same Shot is not picked twice.
- **Stills.** JPGs have no duration and are huge. A Still becomes a Clip only with an explicit duration on the Timeline; scale it down before use. Respect EXIF orientation.
- **Short Sources.** Some MOVs are ~1.2s. Shot detection and Contact Sheet sampling must handle a Source with a single short Shot.
