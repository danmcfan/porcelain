# porcelain

An AI band video editor. Point your own LLM agent (the Harness) at a folder of phone Footage and a Song, give it a Creative Prompt, and it cuts a music video.

`porcelain` is a single static Go binary with no LLM inside. The Harness does the creative work through the Skill in [`skill/porcelain/SKILL.md`](skill/porcelain/SKILL.md) and writes a `timeline.json`; the binary does the deterministic work (probe, validate, render) with ffmpeg and prints JSON for the agent to read.

## Install

1. Install ffmpeg (provides `ffmpeg` and `ffprobe`): `brew install ffmpeg` or `apt-get install ffmpeg`.
2. Build the Skill for your machine and copy it into your agent's skills folder:

   ```sh
   make skill
   cp -R dist/skill/porcelain ~/.claude/skills/
   ```

   Or put a binary from `make dist` on your `PATH`.

## Quickstart

```sh
make build
bin/porcelain doctor
bin/porcelain probe ~/Footage/gig
bin/porcelain timeline validate timeline.json
bin/porcelain render timeline.json -o render.mp4
bin/porcelain export timeline.json --format fcpxml -o timeline.fcpxml
```

The Timeline format is [`skill/porcelain/timeline.schema.json`](skill/porcelain/timeline.schema.json). `analyze-song` is reserved and exits 2 with `not_implemented`.

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

