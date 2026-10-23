# Glossary

Use these terms exactly, in prose and in identifiers. "Avoid" lists synonyms that mean the same thing and must not be used.

## Inputs

- **Footage**: the set of raw media files the user supplies (videos and Stills). _Avoid: assets, media._
- **Source**: one Footage file. _Avoid: clip (for raw files), asset._
- **Still**: a Source that is a photo.
- **Song**: the audio track that drives timing.
- **Creative Prompt**: the user's natural-language direction for the edit. _Avoid: prompt (alone), instructions._
- **Brand Kit**: band name, font files, logo, colors, and default Look.

## Analysis

- **Shot**: a continuous segment within a Source, found by scene-change detection.
- **Proxy**: a low-res, constant-frame-rate, rotation-applied transcode of a Source, used for analysis and preview.
- **Contact Sheet**: a grid image of frames sampled from a Source's Shots, for the Harness to look at. Each Shot starts a new row; long Shots wrap onto more rows so frames stay about 2.5 s apart.
- **Beat**: a detected beat time in the Song.
- **Section**: a labeled span of the Song (intro, verse, chorus, bridge, outro).

## The edit

- **Timeline**: the editable project, stored as `timeline.json`: Song, ordered Clips, Text Layers, Effects, output settings. The single source of truth (ADR 0003). _Avoid: project, edit plan, montage._
- **Clip**: a placement on the Timeline of a Source range (in/out points) at a Timeline time. _Avoid: cut, segment._
- **Text Layer**: a timed piece of on-screen text with a font.
- **Effect**: a visual treatment on one Clip, with an intensity from 0 to 1. To cover a Section, put the Effect on each Clip in it (ADR 0005). _Avoid: filter._
- **Look**: a named set of Effects built into porcelain (e.g. VHS, grain, distortion). The Harness expands a Look into Effects on Clips; the Timeline stores the Effects, not the Look name (ADR 0005).
- **Variation**: another Timeline made from the same Footage and Song.

## Outputs

- **Render**: producing the finished video file from a Timeline.
- **Export**: writing the Timeline in an NLE format (FCPXML, OTIO).

## Runtime

- **Harness**: the user's LLM agent (Claude Code, Codex, ...) that runs the Skill.
- **Skill**: the packaged `SKILL.md` + `porcelain` binary.
- **Fixture**: a tiny ffmpeg-generated test file reproducing a real-footage quirk (ADR 0004).
