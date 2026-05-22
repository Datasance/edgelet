package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew_Interactive(t *testing.T) {
	clearInteractiveEnv(t)
	u := New(Options{ForceTTY: true})
	if u == nil {
		t.Fatal("expected non-nil UI")
	}
	if !u.Interactive() {
		t.Fatal("expected interactive UI with ForceTTY")
	}
}

func TestClearProgressLine(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})

	u.WriteStageLine("stage one")
	u.ClearProgressLine()

	if !strings.Contains(stderr.String(), clearLine) {
		t.Fatalf("expected clear line sequence, got: %q", stderr.String())
	}

	stderr.Reset()
	u = NewWithWriters(nil, &stderr, Options{Quiet: true, ForceTTY: true})
	u.ClearProgressLine()
	if stderr.Len() != 0 {
		t.Fatalf("expected no output in quiet mode, got: %q", stderr.String())
	}

	stderr.Reset()
	u = NewWithWriters(nil, &stderr, Options{})
	u.ClearProgressLine()
	if stderr.Len() != 0 {
		t.Fatalf("expected no output when non-interactive, got: %q", stderr.String())
	}
}
