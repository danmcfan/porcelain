# Coding standards

Judgement-call rules for reviewers. Mechanical rules (formatting, imports, error wrapping style, unused code) belong in golangci-lint config, not here.

- **Edit decisions flow through the Timeline type.** Every Clip, Text Layer, Effect and output setting is expressed in the Timeline and read from it at Render/Export time (ADR 0003). Callers never assemble ad-hoc ffmpeg arguments for edit decisions.
- **One package invokes ffmpeg/ffprobe.** All process execution of ffmpeg and ffprobe goes through a single internal package; other packages call its typed functions.
- **Glossary terms in identifiers.** Names use `GLOSSARY.md` terms (`Source`, `Shot`, `Clip`, `Timeline`), and avoid listed synonyms. A new domain concept gets a glossary entry in the same change.
- **Behaviour changes come with tests against Fixtures.** A media-handling change ships with a test using a Fixture from `scripts/gen-fixtures.sh`; a new real-footage quirk gets a new Fixture (ADR 0004, `docs/footage.md`).
- **CLI output is machine-readable JSON for the Harness.** Results go to stdout as JSON; human logs and progress go to stderr. Errors are JSON with a stable code and a message telling the Harness what to do next.
- **The binary stays deterministic.** No LLM calls, network access or randomness without a seed (ADR 0001). Creative judgement belongs in the Skill.
