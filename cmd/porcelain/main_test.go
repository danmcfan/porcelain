package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestAnalyzeSongErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		exit int
		code string
	}{
		{[]string{"analyze-song"}, exitUsage, "usage"},
		{[]string{"analyze-song", "a.wav", "b.wav"}, exitUsage, "usage"},
		{[]string{"analyze-song", "no-such-song.wav"}, exitError, "invalid_song"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), tc.args, &stdout, &stderr)
		var e cliError
		if err := json.Unmarshal(stderr.Bytes(), &e); err != nil {
			t.Fatalf("%v: stderr is not JSON: %q", tc.args, stderr.String())
		}
		if code != tc.exit || e.Code != tc.code || stdout.Len() != 0 {
			t.Errorf("%v: exit %d, code %q, stdout %q", tc.args, code, e.Code, stdout.String())
		}
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"nope"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
}
