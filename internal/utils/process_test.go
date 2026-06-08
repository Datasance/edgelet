package utils //nolint:revive // legacy package name

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIsAnotherInstanceRunning_selfPIDIsStale(t *testing.T) {
	tmp := t.TempDir()
	orig := VarRun
	VarRun = filepath.Join(tmp, "run")
	t.Cleanup(func() { VarRun = orig })

	pidFile := filepath.Join(VarRun, pidFileName)
	if err := os.MkdirAll(VarRun, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsAnotherInstanceRunning() {
		t.Fatal("expected self PID in pidfile to be treated as stale")
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatal("expected stale pidfile to be removed")
	}
}

func TestIsAnotherInstanceRunning_invalidPIDIsStale(t *testing.T) {
	tmp := t.TempDir()
	orig := VarRun
	VarRun = filepath.Join(tmp, "run")
	t.Cleanup(func() { VarRun = orig })

	pidFile := filepath.Join(VarRun, pidFileName)
	if err := os.MkdirAll(VarRun, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsAnotherInstanceRunning() {
		t.Fatal("expected invalid pidfile content to be treated as stale")
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatal("expected invalid pidfile to be removed")
	}
}
