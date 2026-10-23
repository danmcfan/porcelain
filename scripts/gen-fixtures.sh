#!/usr/bin/env bash
# Generate tiny synthetic Fixtures that reproduce the quirks of real band
# Footage (see docs/footage.md). Usage: scripts/gen-fixtures.sh <outdir>
# Footage goes in <outdir>/footage, the Song in <outdir>/songs.
# Idempotent and safe to run concurrently: builds in a scratch dir, then
# renames into place; skips when <outdir>/.complete matches this script.
set -euo pipefail

out=${1:?usage: gen-fixtures.sh <outdir>}
stamp=$(shasum -a 256 "$0" | cut -d' ' -f1)
done_already() { [[ -f "$out/.complete" && "$(cat "$out/.complete")" == "$stamp" ]]; }
done_already && exit 0

final=$out
mkdir -p "$(dirname "$final")"
work=$(mktemp -d "$final.tmp.XXXXXX")
trap 'rm -rf "$work"' EXIT
out=$work
fx="$out/footage"
mkdir -p "$fx"

ff() { ffmpeg -hide_banner -nostdin -v error -y "$@"; }
src() { echo "testsrc2=size=$1:rate=$2:duration=$3"; }
tone() { echo "sine=frequency=$1:sample_rate=48000:duration=$2"; }

# iPhone-style HEVC portrait: stored 320x180 landscape, -90 display rotation,
# AAC plus a second audio stream.
ff -f lavfi -i "$(src 320x180 30 2)" -f lavfi -i "$(tone 440 2)" -f lavfi -i "$(tone 660 2)" \
  -map 0:v -map 1:a -map 2:a -c:v libx265 -x265-params log-level=none -tag:v hvc1 -pix_fmt yuv420p \
  -c:a aac -ac 2 "$fx/tmp_rot.mov"
# ffmpeg >= 7 has -display_rotation; older ffmpeg takes the legacy rotate tag
# (whose sign is opposite to the display matrix's).
ff -display_rotation:v:0 -90 -i "$fx/tmp_rot.mov" -map 0 -c copy "$fx/IMG_0001.MOV" 2>/dev/null ||
  ff -i "$fx/tmp_rot.mov" -map 0 -c copy -metadata:s:v:0 rotate=90 "$fx/IMG_0001.MOV"
rm "$fx/tmp_rot.mov"

# Landscape H.264 at 30, 60 and 59.94 fps.
ff -f lavfi -i "$(src 320x180 30 2)" -f lavfi -i "$(tone 440 2)" -c:v libx264 -pix_fmt yuv420p -c:a aac "$fx/IMG_0002.MP4"
ff -f lavfi -i "$(src 320x180 60 2)" -f lavfi -i "$(tone 440 2)" -c:v libx264 -pix_fmt yuv420p -c:a aac "$fx/IMG_0003.MP4"
ff -f lavfi -i "$(src 320x180 60000/1001 2)" -f lavfi -i "$(tone 440 2)" -c:v libx264 -pix_fmt yuv420p -c:a aac "$fx/IMG_0004.MP4"

# Byte-identical duplicate, named the way macOS names copies.
cp "$fx/IMG_0002.MP4" "$fx/IMG_0002 2.MP4"

# A Still.
ff -f lavfi -i "testsrc2=size=320x240:duration=1" -frames:v 1 "$fx/IMG_0005.JPG"

# Finder litter that probe must skip.
printf 'junk' > "$fx/.DS_Store"

# Song: ~8s click track at 120 BPM (a 1 kHz click every 0.5 s) over a low drone.
mkdir -p "$out/songs"
ff -f lavfi -i "aevalsrc=0.6*sin(2*PI*1000*t)*lt(mod(t\,0.5)\,0.03)+0.15*sin(2*PI*110*t):s=48000:d=8" \
  -c:a pcm_s16le "$out/songs/click-120bpm.wav"
# A longer Song (16 s at 96 BPM, a click every 0.625 s) for demo Renders of real Footage.
ff -f lavfi -i "aevalsrc=0.6*sin(2*PI*1000*t)*lt(mod(t\,0.625)\,0.03)+0.15*sin(2*PI*82.4*t):s=48000:d=16" \
  -c:a pcm_s16le "$out/songs/click-96bpm-16s.wav"

echo "$stamp" > "$out/.complete"

# Publish. If another run won the race, keep its result.
out=$final
done_already && exit 0
rm -rf "$final"
mv "$work" "$final" 2>/dev/null || done_already
