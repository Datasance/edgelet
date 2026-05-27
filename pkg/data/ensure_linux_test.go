//go:build linux && !cgo

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

func TestBundleHashMatchesInstalled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	oldHash := filepath.Join(dataRoot, "oldhash")
	binDir := filepath.Join(oldHash, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "edgelet"), []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write edgelet: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "containerd-shim-runc-v2"), []byte("shim"), 0755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	if err := os.Symlink(oldHash, filepath.Join(dataRoot, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	if !bundleHashMatchesInstalled(root, "") {
		t.Fatal("empty want hash should accept any ready current bundle")
	}
	if bundleHashMatchesInstalled(root, "newhash") {
		t.Fatal("expected mismatch for different embedded hash")
	}
	if !bundleHashMatchesInstalled(root, "oldhash") {
		t.Fatal("expected match when want hash equals installed")
	}
}

func TestPromoteCurrentBundleRotatesSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	oldDir := filepath.Join(dataRoot, "oldhash")
	newDir := filepath.Join(dataRoot, "newhash")
	for _, dir := range []string{oldDir, newDir} {
		binDir := filepath.Join(dir, "bin")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatalf("mkdir bin: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "edgelet"), []byte("\x7fELF\x02\x01"), 0755); err != nil {
			t.Fatalf("write edgelet: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "containerd-shim-runc-v2"), []byte("shim"), 0755); err != nil {
			t.Fatalf("write shim: %v", err)
		}
	}
	if err := os.Symlink(oldDir, filepath.Join(dataRoot, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	if err := promoteCurrentBundle(root, newDir); err != nil {
		t.Fatalf("promoteCurrentBundle: %v", err)
	}
	current, err := os.Readlink(filepath.Join(dataRoot, "current"))
	if err != nil {
		t.Fatalf("readlink current: %v", err)
	}
	if current != newDir {
		t.Fatalf("current=%q want %q", current, newDir)
	}
	previous, err := os.Readlink(filepath.Join(dataRoot, "previous"))
	if err != nil {
		t.Fatalf("readlink previous: %v", err)
	}
	if previous != oldDir {
		t.Fatalf("previous=%q want %q", previous, oldDir)
	}
}

func writeReadyBundleDir(t *testing.T, dir string) {
	t.Helper()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "edgelet"), []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write edgelet: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "containerd-shim-runc-v2"), []byte("shim"), 0755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
}

func TestTryUseAuthoritativeBundleOverStaleCurrent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	oldDir := filepath.Join(dataRoot, "stalehash")
	newDir := filepath.Join(dataRoot, "wantedhash")
	writeReadyBundleDir(t, oldDir)
	writeReadyBundleDir(t, newDir)
	if err := os.Symlink(oldDir, filepath.Join(dataRoot, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	got, ok, err := tryUseAuthoritativeBundle(root, "wantedhash", newDir)
	if err != nil {
		t.Fatalf("tryUseAuthoritativeBundle: %v", err)
	}
	if !ok {
		t.Fatal("expected authoritative bundle to be used")
	}
	if got != newDir {
		t.Fatalf("got %q want %q", got, newDir)
	}
	current, err := os.Readlink(filepath.Join(dataRoot, "current"))
	if err != nil {
		t.Fatalf("readlink current: %v", err)
	}
	if current != newDir {
		t.Fatalf("current=%q want %q", current, newDir)
	}
}

func TestExtractPrefersEmbedHashDirOverStaleCurrent(t *testing.T) {
	want := EmbeddedBundleHash()
	if want == "" {
		t.Skip("test binary has no embedded bundle")
	}

	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		t.Fatalf("mkdir data root: %v", err)
	}

	oldDir := filepath.Join(dataRoot, "oldhash")
	newDir := filepath.Join(dataRoot, want)
	writeReadyBundleDir(t, oldDir)
	writeReadyBundleDir(t, newDir)
	if err := os.Symlink(oldDir, filepath.Join(dataRoot, "current")); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	got, err := extract(root)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got != newDir {
		t.Fatalf("extract=%q want embed hash dir %q", got, newDir)
	}

	current, err := os.Readlink(filepath.Join(dataRoot, "current"))
	if err != nil {
		t.Fatalf("readlink current: %v", err)
	}
	if current != newDir {
		t.Fatalf("current=%q want %q", current, newDir)
	}
}

func TestExtractUsesCurrentWhenNoEmbed(t *testing.T) {
	if EmbeddedBundleHash() != "" {
		t.Skip("test binary has embedded bundle")
	}

	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	hashDir := filepath.Join(dataRoot, "abc123")
	writeReadyBundleDir(t, hashDir)
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
