package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestStubsReturnNotImplemented(t *testing.T) {
	for _, cmd := range []string{"analyze-song"} {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), []string{cmd}, &stdout, &stderr)
		var e cliError
		if err := json.Unmarshal(stderr.Bytes(), &e); err != nil {
			t.Fatalf("%s: stderr is not JSON: %q", cmd, stderr.String())
		}
		if code != exitNotImplemented || e.Code != "not_implemented" || stdout.Len() != 0 {
			t.Errorf("%s: exit %d, code %q, stdout %q", cmd, code, e.Code, stdout.String())
		}
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"nope"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
}
