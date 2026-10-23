# 0002: ffmpeg and ffprobe are an external runtime dependency

Status: Accepted

## Context

Decoding HEVC, applying rotation, and rendering need ffmpeg. Embedding it would bring GPL/LGPL licensing obligations into our binary and add roughly 80MB.

## Decision

porcelain finds `ffmpeg` and `ffprobe` on `PATH` at runtime. `porcelain doctor` checks they are present and reports versions and missing capabilities as JSON.

## Consequences

- Users install ffmpeg themselves (e.g. `brew install ffmpeg`); the Skill tells the Harness to run `porcelain doctor` first.
- Behaviour can vary across ffmpeg versions; doctor should flag versions or missing codecs/filters we depend on.
- The binary stays small and permissively licensed.
