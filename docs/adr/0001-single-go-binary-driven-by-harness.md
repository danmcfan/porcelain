# 0001: Single Go binary, no embedded LLM, driven by the Harness via a Skill

Status: Accepted

## Context

porcelain needs both creative judgement (choosing Shots, reading the Creative Prompt, pacing an edit) and deterministic media work (probing, detecting Shots, analysing the Song, rendering). Users already run LLM agent Harnesses (Claude Code, Codex, etc.) that can see images and write files. We want anyone to run porcelain without API keys or a vendor lock-in.

## Decision

porcelain ships as one static `porcelain` binary written in Go, plus a Skill (`SKILL.md`). The Harness's LLM does the creative work: it looks at Contact Sheets, interprets the Creative Prompt, and writes the Timeline. The binary does the deterministic work: probe, Shot detection, Contact Sheets, Beat/Section analysis, Timeline validation, Render, Export. The binary embeds no model and needs no API keys.

Alternatives considered:
- Rust: same single-binary distribution, slower iteration.
- Python / Node: no single binary; users manage a runtime and dependencies.
- Embedding a model API: locks users to one vendor and requires keys.

## Consequences

- Every binary command must be callable non-interactively and print machine-readable JSON the Harness can act on.
- Creative quality depends on the user's Harness model; the Skill must guide it well.
- The binary stays testable without any LLM.
