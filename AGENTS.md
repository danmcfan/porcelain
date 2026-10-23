# AGENTS.md

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues for `danmcfan/porcelain`, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Uses the five default triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Where things live

- Product spec: `docs/prd.md`. Vocabulary: `GLOSSARY.md`.
- Real-footage quirks (rotation, audio streams, frame rates, duplicates): `docs/footage.md`.
- Review rules: `CODING_STANDARDS.md`.
- Code: `cmd/` + `internal/`. The Skill the Harness runs: `skill/`.
