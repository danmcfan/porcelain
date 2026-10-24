# porcelain

An AI band video editor. Point your own LLM agent (the Harness) at a folder of phone Footage and a Song, give it a Creative Prompt, and it cuts a music video.

`porcelain` is a single static Go binary with no LLM inside. The Harness does the creative work through the Skill in [`skill/porcelain/SKILL.md`](skill/porcelain/SKILL.md) and writes a `timeline.json`; the binary does the deterministic work (probe, validate, render) with ffmpeg and prints JSON for the agent to read.

## Install

porcelain works with any LLM agent Harness that loads Skills (a folder holding a `SKILL.md`). You build it from source.

1. Install [Go](https://go.dev/dl/) (the version in `go.mod` or newer) and ffmpeg, which provides `ffmpeg` and `ffprobe`: `brew install go ffmpeg`, `apt-get install golang ffmpeg`, or `winget install GoLang.Go ffmpeg`.
2. Clone and build the Skill for your machine:

   ```sh
   git clone https://github.com/danmcfan/porcelain.git
   cd porcelain
   make skill
   ```

3. Copy `dist/skill/porcelain` into the folder your Harness loads Skills from, so you get `<skills folder>/porcelain/SKILL.md`. For example, Claude Code uses `~/.claude/skills/`; for other Harnesses, see their docs for where Skills live.
4. Ask your agent to run `porcelain doctor`, or run `<skills folder>/porcelain/bin/porcelain doctor` yourself. It checks that ffmpeg is found.

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
| `make dist` | cross-compile darwin/linux/windows, amd64 + arm64, into `dist/` |
| `make skill` | assemble `dist/skill/porcelain/` (SKILL.md, schema, host binary) |

