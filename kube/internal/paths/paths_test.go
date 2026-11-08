package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImagesDir(t *testing.T) {
	if ImagesDir != "/var/lib/edgelet-kube/agent/images" {
		t.Fatal(ImagesDir)
	}
}

func TestFromBundleRecordsKubeletCNI(t *testing.T) {
	dir := t.TempDir()
	layout, err := FromBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Socket != ContainerdSocket || layout.Root != ContainerdRoot || layout.State != ContainerdState {
		t.Fatalf("layout %+v", layout)
	}
	if layout.CNIConfDir != CNIConfDir {
		t.Fatalf("cni conf %s", layout.CNIConfDir)
	}
	if layout.CNIBinDir != filepath.Join(dir, "bin") {
		t.Fatalf("cni bin %s", layout.CNIBinDir)
	}
	if layout.BundledBin != filepath.Join(dir, "bin", "aux") {
		t.Fatalf("bundled bin %s", layout.BundledBin)
	}
	args := strings.Join(layout.KubeletCNIArgs(), " ")
	for _, want := range []string{
		"--cni-conf-dir=" + CNIConfDir,
		"--cni-bin-dir=" + layout.CNIBinDir,
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("kubelet args %q missing %s", args, want)
		}
	}
	for _, blocked := range []string{"/opt/cni/bin", "/run/containerd/containerd.sock"} {
		if strings.Contains(args, blocked) || strings.Contains(layout.Socket, blocked) {
			t.Fatalf("layout uses %s", blocked)
		}
	}
}

func TestFromBundleRejectsHostCNI(t *testing.T) {
	if _, err := FromBundle("/opt/cni"); err == nil {
		t.Fatal("accepted the host cni directory")
	}
}

func TestPlaceBundledBins(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	aux := filepath.Join(bin, "aux")
	if err := os.MkdirAll(aux, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cni", "ip", "busybox"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(aux, "xtables-legacy-multi"), []byte("iptables"), 0o755); err != nil {
		t.Fatal(err)
	}

	layout, err := Place(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bridge", "host-local", "loopback", "portmap"} {
		assertLink(t, filepath.Join(bin, name), "cni")
	}
	assertLink(t, filepath.Join(bin, "modprobe"), "busybox")
	assertLink(t, filepath.Join(aux, "iptables"), "xtables-legacy-multi")
	if layout.CNIBinDir != bin || layout.BundledBin != aux {
		t.Fatalf("layout %+v", layout)
	}

	again, err := Place(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.CNIBinDir != layout.CNIBinDir || again.BundledBin != layout.BundledBin {
		t.Fatalf("second place %+v", again)
	}
}

func TestPlaceRejectsWrongLink(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	aux := filepath.Join(bin, "aux")
	if err := os.MkdirAll(aux, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cni", "ip", "busybox"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(aux, "xtables-legacy-multi"), []byte("iptables"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("busybox", filepath.Join(aux, "iptables")); err != nil {
		t.Fatal(err)
	}
	if _, err := Place(dir); err == nil {
		t.Fatal("accepted iptables pointing away from xtables-legacy")
	}
}

func assertLink(t *testing.T, path, target string) {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("%s -> %s, want %s", path, got, target)
	}
}
