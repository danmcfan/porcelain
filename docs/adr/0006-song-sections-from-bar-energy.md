# 0006: Song Sections come from Bar energy; the Harness may relabel

Status: Accepted

## Context

FR-2 asks `analyze-song` for tempo, Beats, downbeats and labeled Sections (intro, verse, chorus, bridge, outro). librosa is not available in Go and the binary takes no new dependencies beyond ffmpeg (ADR 0002). Labeling a Section is partly creative judgement, and that belongs in the Skill (ADR 0001). The three options were: the binary labels Sections fully (self-similarity clustering), the Harness labels them from an energy plot, or a hybrid.

## Decision

Hybrid. The binary measures and proposes; the Harness decides.

- Beats: spectral flux onsets at 10 ms steps, tempo from their autocorrelation (weighted towards 120 BPM to avoid half and double tempo), Beats by dynamic-programming beat tracking (Ellis 2007). Beats are trimmed at either end where they fall on near-silence.
- Downbeats: every fourth Beat (4/4 is assumed), starting on the phase with the strongest onsets. Without a clear accent the first Beat is a Downbeat.
- Bars run from Downbeat to Downbeat, and each carries its energy (RMS relative to the loudest Bar).
- Sections break where the mean loudness of the two Bars on either side differs by at least 3 dB. Sections are at least two Bars long and always start on a Downbeat.
- Labels are a loudness heuristic. A Section within 3 dB of the loudest is a chorus. A quiet first or last Section that is 3 dB quieter than every other quiet Section is the intro or outro. A quiet Section after the second chorus that leads into another chorus is the bridge. Everything else is a verse. A Song with no loudness contrast is one verse.
- The output includes the Bars, so the Harness can relabel or re-split Sections. The Skill tells it to trust what it hears and what the user says over the labels.

## Consequences

- No repetition analysis: a verse and a chorus at the same loudness get the same label. The Harness corrects this.
- Songs in 3/4 or 6/8 get wrong Downbeats and Bars. The Skill tells the Harness to fall back to Beats.
- Beats carry a few milliseconds of onset-window bias, within one frame even at 60 fps.
