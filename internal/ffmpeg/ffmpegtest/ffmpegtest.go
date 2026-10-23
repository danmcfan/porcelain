// Package ffmpegtest provides Fixtures for integration tests. It lives under
// internal/ffmpeg because generating Fixtures runs external processes.
package ffmpegtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
)

var (
	once sync.Once
	dir  string
	err  error
	out  []byte
)

// RepoRoot returns the repository root.
func RepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// Fixtures generates (once, cached under testdata/fixtures) the synthetic
// Fixtures and returns their directory, which holds footage/ and songs/.
// It skips the test when -short is set or ffmpeg is absent.
func Fixtures(t testing.TB) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: skipped with -short")
	}
	if !ffmpeg.Available() {
		t.Skip("ffmpeg/ffprobe not on PATH: install ffmpeg to run integration tests")
	}
	once.Do(func() {
		root := RepoRoot()
		dir = filepath.Join(root, "testdata", "fixtures")
		cmd := exec.Command("bash", filepath.Join(root, "scripts", "gen-fixtures.sh"), dir)
		cmd.Env = os.Environ()
		out, err = cmd.CombinedOutput()
	})
	if err != nil {
		t.Fatalf("gen-fixtures.sh: %v\n%s", err, out)
	}
	return dir
}
