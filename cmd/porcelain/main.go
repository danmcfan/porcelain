// Command porcelain is the deterministic half of an AI band video editor.
// A Harness (the user's LLM agent) drives it through the Skill; every
// subcommand prints JSON to stdout, and errors as JSON to stderr.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/danmcfan/porcelain/internal/analysis"
	"github.com/danmcfan/porcelain/internal/ffmpeg"
	"github.com/danmcfan/porcelain/internal/probe"
	"github.com/danmcfan/porcelain/internal/render"
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
  porcelain version

Not implemented yet (exit 2): analyze-song, export

Proxies, Shots and Contact Sheets are cached in $PORCELAIN_CACHE
(default: the user cache directory), keyed by Source content hash.
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
	case "analyze-song", "export":
		return nil, fail(exitNotImplemented, "not_implemented",
			fmt.Errorf("porcelain %s is not implemented yet", cmd), nil)
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

func renderCmd(ctx context.Context, args []string) (any, error) {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("o", "", "output .mp4 path")
	// Accept the Timeline before or after flags.
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
