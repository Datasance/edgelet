//go:build linux && !cgo

package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet/pkg/dataverify"
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
	if isBundleReady(tmp) {
		t.Fatal("expected not ready without net aux")
	}
	writeNetAuxStubs(t, binDir)
	if !isBundleReady(tmp) {
		t.Fatal("expected bundle ready with fat edgelet, shim, and net aux")
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
	writeNetAuxStubs(t, binDir)
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

func writeNetAuxStubs(t *testing.T, binDir string) {
	t.Helper()
	auxDir := filepath.Join(binDir, "aux")
	if err := os.MkdirAll(auxDir, 0755); err != nil {
		t.Fatalf("mkdir aux: %v", err)
	}
	if err := os.WriteFile(filepath.Join(auxDir, "xtables-legacy-multi"), []byte("\x7fELF"), 0755); err != nil {
		t.Fatalf("write xtables-legacy-multi: %v", err)
	}
	if err := os.Symlink("xtables-legacy-multi", filepath.Join(auxDir, "iptables")); err != nil {
		t.Fatalf("symlink iptables: %v", err)
	}
	for _, name := range []string{"ip", "busybox"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("\x7fELF"), 0755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
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
	writeNetAuxStubs(t, binDir)
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

func TestExtractReusesReadyDir(t *testing.T) {
	want := EmbeddedBundleHash()
	if want == "" {
		t.Skip("test binary has no embedded bundle")
	}

	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	hashDir := filepath.Join(dataRoot, want)
	writeReadyBundleDir(t, hashDir)
	marker := filepath.Join(hashDir, "marker-ready")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	got, err := extract(root)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got != hashDir {
		t.Fatalf("extract=%q want %q", got, hashDir)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("ready dir marker should remain: %v", err)
	}
}

func TestExtractReplacesCorruptDir(t *testing.T) {
	want := EmbeddedBundleHash()
	if want == "" {
		t.Skip("test binary has no embedded bundle")
	}

	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	hashDir := filepath.Join(dataRoot, want)
	binDir := filepath.Join(hashDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "edgelet"), []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write edgelet: %v", err)
	}
	marker := filepath.Join(hashDir, "marker-corrupt")
	if err := os.WriteFile(marker, []byte("stale"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	got, err := extract(root)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got != hashDir {
		t.Fatalf("extract=%q want %q", got, hashDir)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("corrupt dir marker should be replaced, stat err=%v", err)
	}
	if !isBundleReady(hashDir) {
		t.Fatal("expected replaced bundle to be ready")
	}
}

func TestExtractFreshInstall(t *testing.T) {
	want := EmbeddedBundleHash()
	if want == "" {
		t.Skip("test binary has no embedded bundle")
	}

	t.Parallel()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	hashDir := filepath.Join(dataRoot, want)
	if _, err := os.Stat(hashDir); !os.IsNotExist(err) {
		t.Fatalf("hash dir should not exist before extract: %v", err)
	}

	got, err := extract(root)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got != hashDir {
		t.Fatalf("extract=%q want %q", got, hashDir)
	}
	if !isBundleReady(hashDir) {
		t.Fatal("expected fresh extract to produce ready bundle")
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

func writeNetAuxWithNftIptables(t *testing.T, binDir string) {
	t.Helper()
	auxDir := filepath.Join(binDir, "aux")
	if err := os.MkdirAll(auxDir, 0755); err != nil {
		t.Fatalf("mkdir aux: %v", err)
	}
	if err := os.WriteFile(filepath.Join(auxDir, "xtables-legacy-multi"), []byte("\x7fELF"), 0755); err != nil {
		t.Fatalf("write xtables-legacy-multi: %v", err)
	}
	if err := os.WriteFile(filepath.Join(auxDir, "xtables-nft-multi"), []byte("\x7fELF"), 0755); err != nil {
		t.Fatalf("write xtables-nft-multi: %v", err)
	}
	if err := os.Symlink("xtables-nft-multi", filepath.Join(auxDir, "iptables")); err != nil {
		t.Fatalf("symlink iptables to nft: %v", err)
	}
	for _, name := range []string{"ip", "busybox"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("\x7fELF"), 0755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestPinLegacyIptablesAuxRepairsNftSymlink(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	writeNetAuxWithNftIptables(t, binDir)

	if err := dataverify.VerifyNetAux(binDir); err == nil {
		t.Fatal("expected VerifyNetAux to fail with nft iptables symlink")
	}
	if err := pinLegacyIptablesAux(binDir); err != nil {
		t.Fatalf("pinLegacyIptablesAux: %v", err)
	}
	if err := dataverify.VerifyNetAux(binDir); err != nil {
		t.Fatalf("VerifyNetAux after pin: %v", err)
	}
	target, err := os.Readlink(filepath.Join(binDir, "aux", "iptables"))
	if err != nil {
		t.Fatalf("readlink iptables: %v", err)
	}
	if target != "xtables-legacy-multi" {
		t.Fatalf("iptables -> %q want xtables-legacy-multi", target)
	}
}

func TestPinLegacyIptablesAuxNoOpWhenAlreadyLegacy(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	writeNetAuxStubs(t, binDir)
	linkPath := filepath.Join(binDir, "aux", "iptables")
	before, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink before: %v", err)
	}

	if err := pinLegacyIptablesAux(binDir); err != nil {
		t.Fatalf("pinLegacyIptablesAux: %v", err)
	}
	after, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink after: %v", err)
	}
	if before != after {
		t.Fatalf("symlink changed: before=%q after=%q", before, after)
	}
	if err := dataverify.VerifyNetAux(binDir); err != nil {
		t.Fatalf("VerifyNetAux: %v", err)
	}
}

func TestBundleReadyReasonReportsNetAuxFailure(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "edgelet"), []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write edgelet: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "containerd-shim-runc-v2"), []byte("shim"), 0755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	writeNetAuxWithNftIptables(t, binDir)

	reason := bundleReadyReason(tmp)
	if reason == "" {
		t.Fatal("expected non-empty reason for nft iptables symlink")
	}
	if !strings.HasPrefix(reason, "net aux:") {
		t.Fatalf("reason=%q want prefix net aux:", reason)
	}
	if !strings.Contains(reason, "xtables-nft-multi") {
		t.Fatalf("reason=%q should mention xtables-nft-multi", reason)
	}
}

func TestBundleReadyReasonReportsMissingShim(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "edgelet"), []byte("\x7fELF\x02\x01"), 0755); err != nil {
		t.Fatalf("write edgelet: %v", err)
	}

	reason := bundleReadyReason(tmp)
	if reason != "missing containerd-shim-runc-v2" {
		t.Fatalf("reason=%q want missing containerd-shim-runc-v2", reason)
	}
}
