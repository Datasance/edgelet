//go:build linux && full

package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsBundleReadyRequiresFatRuntimeAndShim(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	if isBundleReady(tmp) {
		t.Fatal("empty dir should not be ready")
	}

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "edgelet"), []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write edgelet: %v", err)
	}
	if isBundleReady(tmp) {
		t.Fatal("expected not ready without shim")
	}
	if err := os.WriteFile(filepath.Join(binDir, "containerd-shim-runc-v2"), []byte("shim"), 0755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	if !isBundleReady(tmp) {
		t.Fatal("expected bundle ready with fat edgelet and shim")
	}

	badELF := filepath.Join(binDir, "edgelet")
	if err := os.WriteFile(badELF, []byte("corrupt"), 0755); err != nil {
		t.Fatalf("overwrite edgelet: %v", err)
	}
	if isBundleReady(tmp) {
		t.Fatal("corrupt edgelet should not be ready")
	}
}

func TestRuntimeBinaryUsesExtractDir(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	fatPath := filepath.Join(binDir, "edgelet")
	if err := os.WriteFile(fatPath, []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write edgelet: %v", err)
	}

	setExtractDir(tmp)
	t.Cleanup(func() { setExtractDir("") })

	got, err := RuntimeBinary()
	if err != nil {
		t.Fatalf("RuntimeBinary: %v", err)
	}
	if got != fatPath {
		t.Fatalf("RuntimeBinary=%q want %q", got, fatPath)
	}
}

func TestExtractUsesCurrentSymlinkWithoutEmbed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	hashDir := filepath.Join(dataRoot, "abc123")
	binDir := filepath.Join(hashDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "edgelet"), []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write edgelet: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "containerd-shim-runc-v2"), []byte("shim"), 0755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	if err := os.Symlink(hashDir, filepath.Join(dataRoot, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	got, err := extract(root)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got != hashDir {
		t.Fatalf("extract=%q want %q", got, hashDir)
	}
}
