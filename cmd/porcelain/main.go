// Command porcelain is the deterministic half of an AI band video editor.
// A Harness (the user's LLM agent) drives it through the Skill; every
// subcommand prints JSON to stdout, and errors as JSON to stderr.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/danmcfan/porcelain/internal/analysis"
	"github.com/danmcfan/porcelain/internal/brandkit"
	"github.com/danmcfan/porcelain/internal/export"
	"github.com/danmcfan/porcelain/internal/ffmpeg"
	"github.com/danmcfan/porcelain/internal/probe"
	"github.com/danmcfan/porcelain/internal/render"
	"github.com/danmcfan/porcelain/internal/song"
	"github.com/danmcfan/porcelain/internal/timeline"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// Exit codes.
const (
	exitOK             = 0
	exitError          = 1
	exitNotImplemented = 2
	exitUsage          = 64
)

const usage = `porcelain: deterministic video editing for an LLM Harness. Output is JSON.

Usage:
  porcelain doctor                         check ffmpeg/ffprobe
  porcelain probe <dir|file>...            describe each Source in the Footage
  porcelain shots <dir|file>...            split each video Source into Shots
  porcelain contact-sheet <dir|file>...    grid images of each Source's Shots
  porcelain timeline validate <file.json>  check a Timeline against the schema and Footage
  porcelain render <timeline.json> -o out.mp4
  porcelain export <timeline.json> --format fcpxml|otio -o <file>
  porcelain analyze-song <song>            tempo, Beats, Downbeats, Bar energy, Sections
  porcelain brand-kit save <kit.json>      check and save the Brand Kit for every new Timeline
  porcelain brand-kit show                 print the saved Brand Kit
  porcelain looks                          list the built-in Looks and their Effects
  porcelain version

Proxies, Shots and Contact Sheets are cached in $PORCELAIN_CACHE
(default: the user cache directory), keyed by Source content hash.
The Brand Kit is saved to $PORCELAIN_BRAND_KIT
(default: porcelain/brandkit.json in the user config directory).
`

// cliError is the JSON shape of every error.
type cliError struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details any    `json:"details,omitempty"`
}

type failure struct {
	exit int
	body cliError
}

func (f *failure) Error() string { return f.body.Error }

func fail(exit int, code string, err error, details any) error {
	return &failure{exit: exit, body: cliError{Error: err.Error(), Code: code, Details: details}}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	result, err := dispatch(ctx, args)
	if err != nil {
		var f *failure
		if !errors.As(err, &f) {
			f = &failure{exit: exitError, body: cliError{Error: err.Error(), Code: "error"}}
		}
		writeJSON(stderr, f.body)
		return f.exit
	}
	writeJSON(stdout, result)
	return exitOK
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func dispatch(ctx context.Context, args []string) (any, error) {
	if len(args) == 0 {
		return nil, usageErr("missing subcommand")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "doctor":
		return doctor(ctx)
	case "probe":
		return probeCmd(ctx, rest)
	case "timeline":
		if len(rest) == 0 || rest[0] != "validate" {
			return nil, usageErr("usage: porcelain timeline validate <timeline.json>")
		}
		return validateCmd(ctx, rest[1:])
	case "render":
		return renderCmd(ctx, rest)
	case "shots":
		return shotsCmd(ctx, rest)
	case "contact-sheet":
		return contactSheetCmd(ctx, rest)
	case "export":
		return exportCmd(ctx, rest)
	case "analyze-song":
		return analyzeSongCmd(ctx, rest)
	case "brand-kit":
		return brandKitCmd(rest)
	case "looks":
		if len(rest) != 0 {
			return nil, usageErr("usage: porcelain looks")
		}
		return map[string]any{"looks": timeline.Looks}, nil
	case "version", "--version":
		return map[string]string{"version": version}, nil
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(os.Stdout, usage)
		os.Exit(exitOK)
	}
	return nil, usageErr(fmt.Sprintf("unknown subcommand %q", cmd))
}

func usageErr(msg string) error {
	return fail(exitUsage, "usage", errors.New(msg), usage)
}

func doctor(ctx context.Context) (any, error) {
	tools := ffmpeg.Check(ctx)
	ok := true
	for _, t := range tools {
		ok = ok && t.Found && t.Error == ""
	}
	res := map[string]any{"ok": ok, "version": version, "tools": tools}
	if !ok {
		return nil, fail(exitError, "missing_dependency",
			errors.New("ffmpeg and ffprobe must be installed and on PATH"), res)
	}
	return res, nil
}

func probeCmd(ctx context.Context, args []string) (any, error) {
	if len(args) == 0 {
		return nil, usageErr("usage: porcelain probe <dir|file>...")
	}
	paths, err := probe.Expand(args)
	if err != nil {
		return nil, err
	}
	sources := probe.Probe(ctx, paths)
	return map[string]any{"sources": sources}, nil
}

func analyzeSongCmd(ctx context.Context, args []string) (any, error) {
	if len(args) != 1 {
		return nil, usageErr("usage: porcelain analyze-song <song>")
	}
	if _, err := os.Stat(args[0]); err != nil {
		return nil, fail(exitError, "invalid_song", fmt.Errorf("%w: check the Song path", err), nil)
	}
	a, err := song.Analyze(ctx, args[0])
	if err != nil {
		return nil, fail(exitError, "invalid_song",
			fmt.Errorf("%w: the Song must be an audio file at least one second long", err), nil)
	}
	return a, nil
}

// skipped names a Source a command did not analyse, and why.
type skipped struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// eachVideo probes the Footage named by args and calls fn for every
// distinct video Source, collecting the rest (and fn's failures) as skipped.
func eachVideo(ctx context.Context, cmd string, args []string, fn func(analysis.Cache, probe.Source) error) ([]skipped, error) {
	if len(args) == 0 {
		return nil, usageErr(fmt.Sprintf("usage: porcelain %s <dir|file>...", cmd))
	}
	paths, err := probe.Expand(args)
	if err != nil {
		return nil, err
	}
	cache, err := analysis.DefaultCache()
	if err != nil {
		return nil, err
	}
	skips := []skipped{}
	for _, src := range probe.Probe(ctx, paths) {
		var reason string
		switch {
		case src.Error != "":
			reason = src.Error
		case src.DuplicateOf != "":
			reason = "duplicate of " + src.DuplicateOf
		case src.Kind == probe.KindStill:
			reason = "Still: view the file directly"
		case src.Kind != probe.KindVideo:
			reason = fmt.Sprintf("not a video Source (kind %s)", src.Kind)
		default:
			if err := fn(cache, src); err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				reason = err.Error()
			}
		}
		if reason != "" {
			skips = append(skips, skipped{Source: src.Path, Reason: reason})
		}
	}
	return skips, nil
}

func shotsCmd(ctx context.Context, args []string) (any, error) {
	shots := []analysis.Shot{}
	skips, err := eachVideo(ctx, "shots", args, func(c analysis.Cache, src probe.Source) error {
		s, err := analysis.Shots(ctx, c, src)
		if err != nil {
			return err
		}
		shots = append(shots, s...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"shots": shots, "skipped": skips}, nil
}

func contactSheetCmd(ctx context.Context, args []string) (any, error) {
	sheets := []analysis.ContactSheet{}
	skips, err := eachVideo(ctx, "contact-sheet", args, func(c analysis.Cache, src probe.Source) error {
		shots, err := analysis.Shots(ctx, c, src)
		if err != nil {
			return err
		}
		s, err := analysis.ContactSheets(ctx, c, src, shots)
		if err != nil {
			return err
		}
		sheets = append(sheets, s...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"contact_sheets": sheets, "skipped": skips}, nil
}

func validateCmd(ctx context.Context, args []string) (any, error) {
	if len(args) != 1 {
		return nil, usageErr("usage: porcelain timeline validate <timeline.json>")
	}
	t, err := timeline.Load(args[0])
	if err != nil {
		return nil, fail(exitError, "invalid_timeline", err, nil)
	}
	if ps := timeline.Validate(t, probe.NewMedia(ctx)); len(ps) > 0 {
		return nil, fail(exitError, "invalid_timeline",
			fmt.Errorf("%d problem(s) in %s", len(ps), args[0]), ps)
	}
	return map[string]any{"valid": true, "duration": t.Duration(), "clips": len(t.Clips)}, nil
}

// parseInterleaved parses fs, accepting positional arguments before or
// after flags.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var positional []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, usageErr(err.Error())
		}
		args = fs.Args()
		if len(args) > 0 {
			positional = append(positional, args[0])
			args = args[1:]
		}
	}
	return positional, nil
}

func renderCmd(ctx context.Context, args []string) (any, error) {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	out := fs.String("o", "", "output .mp4 path")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return nil, err
	}
	if len(positional) != 1 || *out == "" {
		return nil, usageErr("usage: porcelain render <timeline.json> -o out.mp4")
	}
	t, err := timeline.Load(positional[0])
	if err != nil {
		return nil, fail(exitError, "invalid_timeline", err, nil)
	}
	res, err := render.Render(ctx, t, *out)
	var inv *render.InvalidError
	switch {
	case errors.As(err, &inv):
		return nil, fail(exitError, "invalid_timeline", err, inv.Problems)
	case errors.Is(err, render.ErrNotSupported):
		return nil, fail(exitNotImplemented, "not_implemented", err, nil)
	case err != nil:
		return nil, fail(exitError, "render_failed", err, nil)
	}
	return res, nil
}

func brandKitCmd(args []string) (any, error) {
	const use = "usage: porcelain brand-kit save <kit.json> | porcelain brand-kit show"
	path, err := brandkit.DefaultPath()
	if err != nil {
		return nil, err
	}
	var k *brandkit.BrandKit
	switch {
	case len(args) == 2 && args[0] == "save":
		if k, err = brandkit.Load(args[1]); err == nil {
			err = brandkit.Save(k, path)
		}
	case len(args) == 1 && args[0] == "show":
		k, err = brandkit.Load(path)
	default:
		return nil, usageErr(use)
	}
	var inv *brandkit.InvalidError
	switch {
	case errors.As(err, &inv):
		return nil, fail(exitError, "invalid_brand_kit", err, inv.Problems)
	case errors.Is(err, brandkit.ErrNotSaved) && args[0] == "show":
		return nil, fail(exitError, "no_brand_kit",
			fmt.Errorf("%w: ask the user for band name, font files, logo, colors and Look, then run porcelain brand-kit save", err), nil)
	case err != nil:
		return nil, fail(exitError, "invalid_brand_kit", err, nil)
	}
	return map[string]any{"path": path, "brand_kit": k}, nil
}

func exportCmd(ctx context.Context, args []string) (any, error) {
	const use = "usage: porcelain export <timeline.json> --format fcpxml|otio -o <file>"
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	format := fs.String("format", "", "fcpxml or otio")
	out := fs.String("o", "", "output path")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return nil, err
	}
	writers := map[string]func(*timeline.Timeline, export.Sources, io.Writer) (*export.Report, error){
		"fcpxml": export.FCPXML,
		"otio":   export.OTIO,
	}
	write, ok := writers[*format]
	if len(positional) != 1 || *out == "" || !ok {
		return nil, usageErr(use)
	}
	t, err := timeline.Load(positional[0])
	if err != nil {
		return nil, fail(exitError, "invalid_timeline", err, nil)
	}
	media := probe.NewMedia(ctx)
	if ps := timeline.Validate(t, media); len(ps) > 0 {
		return nil, fail(exitError, "invalid_timeline",
			fmt.Errorf("%d problem(s) in %s", len(ps), positional[0]), ps)
	}
	var buf bytes.Buffer
	rep, err := write(t, media, &buf)
	if err == nil {
		err = os.WriteFile(*out, buf.Bytes(), 0o644)
	}
	if err != nil {
		return nil, fail(exitError, "export_failed", err, nil)
	}
	return map[string]any{"output": *out, "format": rep.Format, "clips": rep.Clips,
		"duration": rep.Duration, "dropped": rep.Dropped}, nil
}
