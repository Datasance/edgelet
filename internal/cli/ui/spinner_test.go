package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestSpinner_QuietPrintsPlainLine(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{Quiet: true})
	spin := u.StartSpinner("applying manifest")
	spin.Stop()

	out := stderr.String()
	if !strings.Contains(out, "applying manifest") {
		t.Fatalf("expected plain message, got: %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("did not expect ANSI spinner output in quiet mode, got: %q", out)
	}
}

func TestSpinner_NonInteractivePrintsPlainLine(t *testing.T) {
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{})
	spin := u.StartSpinner("working")
	spin.Stop()

	if !strings.Contains(stderr.String(), "working") {
		t.Fatalf("expected plain line, got: %q", stderr.String())
	}
}

func TestSpinner_SetSuffixWhileRunning(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	spin := u.StartSpinner("initial")
	spin.SetSuffix("updated")
	spin.Stop()
}

func TestSpinner_SetSuffixNilSafe(t *testing.T) {
	var s *Spinner
	s.SetSuffix("ignored")
}

func TestSpinner_PauseSpinner(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	spin := u.StartSpinner("working")
	if !u.PauseSpinner(spin) {
		t.Fatal("expected PauseSpinner to report running spinner")
	}
	if u.PauseSpinner(nil) {
		t.Fatal("expected PauseSpinner(nil) to return false")
	}
}

func TestWriteSuccessAndErrorClearLine(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	u.WriteStageLine("in progress")
	u.WriteSuccess("done")

	out := stderr.String()
	if !strings.Contains(out, "✔ done") {
		t.Fatalf("expected success marker, got: %q", out)
	}
	if !strings.Contains(out, green) {
		t.Fatalf("expected green ANSI for success, got: %q", out)
	}

	stderr.Reset()
	u.WriteError("failed")
	out = stderr.String()
	if !strings.Contains(out, "✘ failed") {
		t.Fatalf("expected error marker, got: %q", out)
	}
	if !strings.Contains(out, red) {
		t.Fatalf("expected red ANSI for error, got: %q", out)
	}
}

func TestWriteSuccess_NoColorPlainMarker(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true, NoColor: true})
	u.WriteSuccess("done")

	out := stderr.String()
	if !strings.Contains(out, "✔ done") {
		t.Fatalf("expected plain success marker, got: %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected no ANSI with NoColor, got: %q", out)
	}
}

func TestStopSpinner_StopsActiveSpinner(t *testing.T) {
	clearInteractiveEnv(t)
	var stderr bytes.Buffer
	u := NewWithWriters(nil, &stderr, Options{ForceTTY: true})
	spin := u.StartSpinner("working")
	if u.activeSpinner == nil {
		t.Fatal("expected active spinner")
	}
	u.StopSpinner()
	if u.activeSpinner != nil {
		t.Fatal("expected active spinner cleared")
	}
	spin.Stop()
}
