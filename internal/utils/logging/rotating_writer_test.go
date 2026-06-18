package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriter_SetLimitsNoRotation(t *testing.T) {
	dir := t.TempDir()
	w, err := NewRotatingWriter(dir, BasenameControlPlane, 1024*1024, 5, false)
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	if _, err := w.Write([]byte("seed\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	w.SetLimits(512*1024, 3)
	maxSize, maxBackups := w.Limits()
	if maxSize != 512*1024 || maxBackups != 3 {
		t.Fatalf("SetLimits: got maxSize=%d maxBackups=%d", maxSize, maxBackups)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, BasenameControlPlane+".1.log")); !os.IsNotExist(err) {
		t.Fatalf("SetLimits must not rotate files, stat .1.log: %v", err)
	}
}

func TestRotatingWriter_DualBasenameIndependent(t *testing.T) {
	dir := t.TempDir()

	wControl, err := NewRotatingWriter(dir, BasenameControlPlane, 1024*1024, 3, false)
	if err != nil {
		t.Fatalf("NewRotatingWriter(control): %v", err)
	}
	if _, err := wControl.Write([]byte("control plane log\n")); err != nil {
		t.Fatalf("Write(control): %v", err)
	}
	if err := wControl.Close(); err != nil {
		t.Fatalf("Close(control): %v", err)
	}

	wData, err := NewRotatingWriter(dir, BasenameDataPlane, 1024*1024, 3, false)
	if err != nil {
		t.Fatalf("NewRotatingWriter(data): %v", err)
	}
	if _, err := wData.Write([]byte("data plane log\n")); err != nil {
		t.Fatalf("Write(data): %v", err)
	}
	if err := wData.Close(); err != nil {
		t.Fatalf("Close(data): %v", err)
	}

	controlPath := filepath.Join(dir, BasenameControlPlane+".0.log")
	dataPath := filepath.Join(dir, BasenameDataPlane+".0.log")
	assertFileContains(t, controlPath, "control plane log")
	assertFileContains(t, dataPath, "data plane log")

	// Simulate control-plane restart: rotate only the edgelet series.
	wControl, err = NewRotatingWriter(dir, BasenameControlPlane, 1024*1024, 3, true)
	if err != nil {
		t.Fatalf("NewRotatingWriter(control restart): %v", err)
	}
	if _, err := wControl.Write([]byte("new control plane log\n")); err != nil {
		t.Fatalf("Write(control restart): %v", err)
	}
	if err := wControl.Close(); err != nil {
		t.Fatalf("Close(control restart): %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, BasenameControlPlane+".1.log")); err != nil {
		t.Fatalf("expected rotated control-plane backup: %v", err)
	}
	assertFileContains(t, controlPath, "new control plane log")
	assertFileContains(t, filepath.Join(dir, BasenameControlPlane+".1.log"), "control plane log")

	if _, err := os.Stat(filepath.Join(dir, BasenameDataPlane+".1.log")); !os.IsNotExist(err) {
		t.Fatalf("data-plane series must not rotate with control-plane basename, stat .1.log: %v", err)
	}
	assertFileContains(t, dataPath, "data plane log")
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s: want substring %q, got %q", path, want, string(data))
	}
}
