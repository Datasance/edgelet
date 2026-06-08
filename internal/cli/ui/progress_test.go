package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestProgress_PersistingToPulling_NoCorruption(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})

	u.WriteStageLine(FormatDeployStageLine("persisting"))
	u.WriteStageLine(FormatDeployStageLine("pulling"))

	final := finalRenderedLine(stderr.String())
	if strings.Contains(final, "(pulling)ng)") {
		t.Fatalf("expected no corrupted persisting suffix on final line, got: %q", final)
	}
	if final != FormatDeployStageLine("pulling") {
		t.Fatalf("expected final line %q, got %q", FormatDeployStageLine("pulling"), final)
	}
}

func finalRenderedLine(rendered string) string {
	parts := strings.Split(rendered, clearLine)
	return parts[len(parts)-1]
}

func TestProgress_InteractiveUsesClearLine(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})

	u.WriteStageLine(FormatDeployStageLine("parsing"))

	if !strings.Contains(stderr.String(), "\x1b[K") {
		t.Fatalf("expected clear-to-EOL sequence in interactive output, got: %q", stderr.String())
	}
}

func TestProgress_NoColorEnvDisablesANSI(t *testing.T) {
	clearInteractiveEnv(t)
	t.Setenv("NO_COLOR", "1")
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	u.WriteStageLine(FormatDeployStageLine("pulling"))
	if strings.Contains(stderr.String(), "\x1b[") {
		t.Fatalf("expected no ANSI escapes with NO_COLOR=1, got: %q", stderr.String())
	}
}

func TestProgress_CIEnvUsesNonInteractiveProgress(t *testing.T) {
	clearInteractiveEnv(t)
	t.Setenv("CI", "true")
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	u.WriteStageLine(FormatDeployStageLine("parsing"))
	u.WriteStageLine(FormatDeployStageLine("pulling"))
	if strings.Contains(stderr.String(), "\x1b[K") {
		t.Fatalf("expected non-interactive progress with CI=true, got: %q", stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected one line per stage with CI=true, got %d: %q", len(lines), stderr.String())
	}
}

func TestProgress_NonInteractiveOneLinePerStage(t *testing.T) {
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{})

	u.WriteStageLine(FormatDeployStageLine("parsing"))
	u.WriteStageLine(FormatDeployStageLine("pulling"))

	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two stage lines, got %d: %q", len(lines), stderr.String())
	}
	if strings.Contains(stderr.String(), "\x1b[K") {
		t.Fatal("did not expect ANSI clear in non-interactive output")
	}
}

func TestFormatDeployStageLine(t *testing.T) {
	if got := FormatDeployStageLine("pulling"); !strings.Contains(got, "(pulling)") {
		t.Fatalf("unexpected stage line: %s", got)
	}
	if got := FormatDeployStageLine(""); got != "applying microservice manifest..." {
		t.Fatalf("unexpected empty stage line: %s", got)
	}
	if got := FormatDeployStageLine("<unknown>"); got != "applying microservice manifest..." {
		t.Fatalf("unexpected unknown stage line: %s", got)
	}
}

func TestFormatControlPlaneStageLine(t *testing.T) {
	if got := FormatControlPlaneStageLine("pulling"); !strings.Contains(got, "control plane") || !strings.Contains(got, "(pulling)") {
		t.Fatalf("unexpected stage line: %s", got)
	}
	if got := FormatControlPlaneStageLine(""); got != "applying control plane manifest..." {
		t.Fatalf("unexpected empty stage line: %s", got)
	}
}

func TestWritePercent_DoneAddsNewlineInteractive(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	u.WritePercent("pulling image", 100, true)
	if !strings.HasSuffix(stderr.String(), "\n") {
		t.Fatalf("expected trailing newline when done, got: %q", stderr.String())
	}
}

func TestWritePercent_InteractiveInProgress(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	u.WritePercent("pulling image", 42, false)
	out := stderr.String()
	if !strings.Contains(out, "\x1b[K") {
		t.Fatalf("expected clear-to-EOL in interactive percent output, got: %q", out)
	}
	if strings.HasSuffix(out, "\n") {
		t.Fatalf("did not expect trailing newline while in progress, got: %q", out)
	}
}

func TestWritePercent_NonInteractiveDone(t *testing.T) {
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{})
	u.WritePercent("pulling image", 100, true)
	if !strings.Contains(stderr.String(), "pulling image: 100%") {
		t.Fatalf("expected percent line in non-interactive mode, got: %q", stderr.String())
	}
}

func TestWritePercent_NonInteractiveInProgressSilent(t *testing.T) {
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{})
	u.WritePercent("pulling image", 50, false)
	if stderr.Len() != 0 {
		t.Fatalf("expected no output for in-progress non-interactive percent, got: %q", stderr.String())
	}
}

func TestWritePercent_Quiet(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{Quiet: true, ForceTTY: true})
	u.WritePercent("pulling image", 100, true)
	if stderr.Len() != 0 {
		t.Fatalf("expected no output in quiet mode, got: %q", stderr.String())
	}
}

func TestWritePercent_Clamp(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	u.WritePercent("pull", -5, true)
	if !strings.Contains(stderr.String(), "pull:   0%") {
		t.Fatalf("expected clamped 0%%, got: %q", stderr.String())
	}
	stderr.Reset()
	u.WritePercent("pull", 150, true)
	if !strings.Contains(stderr.String(), "pull: 100%") {
		t.Fatalf("expected clamped 100%%, got: %q", stderr.String())
	}
}
