# 0007: Text Layers are rasterized in Go; the Brand Kit lives in the user config directory

Status: Accepted

## Context

FR-5 asks for Text Layers drawn in the band's own TTF/OTF fonts, and a Brand Kit saved once and applied to every new Timeline. ffmpeg's `drawtext` filter needs a build with libfreetype, and common builds (including Homebrew's default on the development machine) have neither `drawtext` nor `subtitles`. ADR 0002 lets porcelain require ffmpeg, but not a particular build of it.

## Decision

- Render rasterizes each Text Layer in Go (`golang.org/x/image/font/opentype`, pure Go so CGO stays off) to a full-frame transparent PNG. ffmpeg's `overlay` filter lays it over the frame while `start <= t < end`.
- Each Text Layer names its font file. A missing or unparseable font is a validation error, so a Render never silently falls back to another font.
- The Brand Kit is `brandkit.json` in porcelain's user config directory (`$PORCELAIN_BRAND_KIT` overrides). `porcelain brand-kit save` checks it and stores it with absolute paths; `porcelain brand-kit show` prints it. Render never reads the Brand Kit: the Skill copies its fonts, colors and default Look into each new Timeline, so the Timeline stays the single source of truth (ADR 0003).

## Consequences

- Text is static: no per-frame animation (fades, kinetic type) without rasterizing more frames.
- A Timeline depends on the font files staying where they are, as it does for Footage.
- FCPXML and OTIO exports still drop Text Layers.
