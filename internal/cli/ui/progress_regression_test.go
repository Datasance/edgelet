package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestClearProgressLineInteractive(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	u.WriteStageLine(FormatDeployStageLine("parsing"))
	u.ClearProgressLine()
	if !strings.HasSuffix(stderr.String(), clearLine) {
		t.Fatalf("expected clear line sequence, got: %q", stderr.String())
	}
}

func TestWriteStageLine_SkipsDuplicate(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	line := FormatDeployStageLine("pulling")
	u.WriteStageLine(line)
	before := stderr.String()
	u.WriteStageLine(line)
	if stderr.String() != before {
		t.Fatal("expected duplicate stage line to be skipped")
	}
}

func TestWriteStageLine_QuietNoOutput(t *testing.T) {
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{Quiet: true, ForceTTY: true})
	u.WriteStageLine(FormatDeployStageLine("pulling"))
	if stderr.Len() != 0 {
		t.Fatalf("expected quiet mode to suppress stage output, got: %q", stderr.String())
	}
}

func TestRawOverwriteWithoutClearLeavesCorruption(t *testing.T) {
	long := FormatDeployStageLine("persisting")
	short := FormatDeployStageLine("pulling")
	visible := simulateTerminalOverwrite(long, short)
	if !strings.Contains(visible, "(pulling)ng)") {
		t.Fatalf("expected corrupted visible line without clearLine, got: %q", visible)
	}
}

func simulateTerminalOverwrite(previous, next string) string {
	if len(next) >= len(previous) {
		return next
	}
	return next + previous[len(next):]
}
