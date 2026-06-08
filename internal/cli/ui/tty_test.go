package ui

import (
	"bytes"
	"os"
	"testing"
)

func clearInteractiveEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CI", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
}

func TestIsInteractive_PipedWriterNotTTY(t *testing.T) {
	var buf bytes.Buffer
	if IsInteractive(&buf, Options{}) {
		t.Fatal("expected non-interactive for non-file writer")
	}
}

func TestIsInteractive_Quiet(t *testing.T) {
	if IsInteractive(nil, Options{Quiet: true, ForceTTY: true}) {
		t.Fatal("expected quiet to disable interactivity")
	}
}

func TestIsInteractive_ForceTTY(t *testing.T) {
	clearInteractiveEnv(t)
	if !IsInteractive(nil, Options{ForceTTY: true}) {
		t.Fatal("expected ForceTTY to enable interactivity")
	}
}

func TestIsInteractive_NoColor(t *testing.T) {
	clearInteractiveEnv(t)
	t.Setenv("NO_COLOR", "1")
	if IsInteractive(nil, Options{}) {
		t.Fatal("expected NO_COLOR to disable interactivity")
	}
}

func TestIsInteractive_CI(t *testing.T) {
	clearInteractiveEnv(t)
	t.Setenv("CI", "true")
	if IsInteractive(nil, Options{ForceTTY: false}) {
		t.Fatal("expected CI=true to disable interactivity")
	}
}

func TestIsInteractive_TermDumb(t *testing.T) {
	clearInteractiveEnv(t)
	t.Setenv("TERM", "dumb")
	if IsInteractive(nil, Options{}) {
		t.Fatal("expected TERM=dumb to disable interactivity")
	}
}

func TestIsInteractive_OptionsNoColor(t *testing.T) {
	if IsInteractive(nil, Options{NoColor: true, ForceTTY: true}) {
		t.Fatal("expected Options.NoColor to disable interactivity")
	}
}

func TestIsInteractive_OsFileNotTerminal(t *testing.T) {
	clearInteractiveEnv(t)
	f, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = f.Close()
	}()
	if IsInteractive(f, Options{}) {
		t.Fatal("expected non-interactive for regular file writer")
	}
}

func TestIsInteractive_OsFileTerminal(t *testing.T) {
	clearInteractiveEnv(t)
	f, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = f.Close()
	}()

	prev := isTerminalFile
	isTerminalFile = func(*os.File) bool { return true }
	t.Cleanup(func() { isTerminalFile = prev })

	if !IsInteractive(f, Options{}) {
		t.Fatal("expected interactive when stderr is a terminal")
	}
}

func TestIsInteractive_EnvTruthyVariants(t *testing.T) {
	for _, val := range []string{"yes", "TRUE", " 1 "} {
		t.Run(val, func(t *testing.T) {
			clearInteractiveEnv(t)
			t.Setenv("NO_COLOR", val)
			if IsInteractive(nil, Options{}) {
				t.Fatalf("expected NO_COLOR=%q to disable interactivity", val)
			}
		})
	}
}

func TestIsInteractive_TermDumbCaseInsensitive(t *testing.T) {
	clearInteractiveEnv(t)
	t.Setenv("TERM", "DUMB")
	if IsInteractive(nil, Options{}) {
		t.Fatal("expected TERM=DUMB to disable interactivity")
	}
}

func TestIsTruthyEnv(t *testing.T) {
	for _, val := range []string{"1", "true", "yes", "TRUE", " Yes "} {
		if !isTruthyEnv(val) {
			t.Fatalf("expected %q to be truthy", val)
		}
	}
	for _, val := range []string{"", "0", "false", "no", "maybe"} {
		if isTruthyEnv(val) {
			t.Fatalf("expected %q to be falsy", val)
		}
	}
}
