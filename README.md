# porcelain

An AI band video editor. Point your own LLM agent (the Harness) at a folder of phone Footage and a Song, give it a Creative Prompt, and it cuts a music video.

`porcelain` is a single static Go binary with no LLM inside. The Harness does the creative work through the Skill in [`skill/porcelain/SKILL.md`](skill/porcelain/SKILL.md) and writes a `timeline.json`; the binary does the deterministic work (probe, validate, render) with ffmpeg and prints JSON for the agent to read.

## Install

porcelain works with any LLM agent Harness that loads Skills (a folder holding a `SKILL.md`).

1. Install ffmpeg (provides `ffmpeg` and `ffprobe`): `brew install ffmpeg`, `apt-get install ffmpeg`, or `winget install ffmpeg`.
2. From the [latest release](https://github.com/danmcfan/porcelain/releases/latest), download `porcelain-skill-<os>-<arch>.zip` for your machine (`darwin` = macOS; `arm64` = Apple Silicon). Check it against `SHA256SUMS` if you like.
3. Unzip it into the folder your Harness loads Skills from, so you get `<skills folder>/porcelain/SKILL.md`. For example, Claude Code uses `~/.claude/skills/`; for other Harnesses, see their docs for where Skills live.
4. On macOS, if the binary is blocked as unsigned, run `xattr -d com.apple.quarantine <skills folder>/porcelain/bin/porcelain`.
5. Ask your agent to run `porcelain doctor`, or run `<skills folder>/porcelain/bin/porcelain doctor` yourself. It checks that ffmpeg is found.

The bare binaries (`porcelain-<os>-<arch>`) are also attached to each release if you want `porcelain` on your `PATH`.

To build from source instead: `make skill` assembles `dist/skill/porcelain` for this machine.

## Quickstart

```sh
make build
bin/porcelain doctor
bin/porcelain probe ~/Footage/gig
bin/porcelain timeline validate timeline.json
bin/porcelain render timeline.json -o render.mp4
bin/porcelain export timeline.json --format fcpxml -o timeline.fcpxml
```

The Timeline format is [`skill/porcelain/timeline.schema.json`](skill/porcelain/timeline.schema.json).

## Development

| Command | Does |
| --- | --- |
| `make build` | build `bin/porcelain` |
| `make test` | unit + integration tests (integration tests need ffmpeg; they generate Fixtures into `testdata/fixtures/`) |
| `make test-short` | unit tests only |
| `make lint` | golangci-lint, including the rule that only `internal/ffmpeg` may use `os/exec` |
| `make fixtures` | generate the synthetic Fixtures |
| `make release` | build every platform's binary and Skill zip, plus `SHA256SUMS`, into `dist/release/` |
| `make publish VERSION=vX.Y.Z` | tag, push the tag and create a GitHub Release from `dist/release/` |
| `make dist` | cross-compile darwin/linux/windows, amd64 + arm64, into `dist/` |
| `make skill` | assemble `dist/skill/porcelain/` (SKILL.md, schema, host binary) |

