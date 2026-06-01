//go:build linux && !cgo

package data

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/pkg/datadir"
)

// prependRuntimePath puts bundled CNI plugins and userland net aux ahead of PATH so
// CNI bridge/portmap and modprobe work on minimal hosts without distro iptables/ip.
// /usr/local/bin precedes bundle bin/ so the thin CLI wins over the fat edgelet ELF name.
func prependRuntimePath(bundleDir string) error {
	root, err := datadir.BundleRoot("")
	if err != nil {
		return err
	}
	cniPath := filepath.Join(root, "cni")
	binDir := filepath.Join(bundleDir, "bin")
	auxDir := filepath.Join(binDir, "aux")

	seen := make(map[string]struct{})
	var parts []string
	for _, p := range []string{
		cniPath,
		auxDir,
		"/usr/local/bin",
		binDir,
		constants.EdgeletContainerdBinDir,
		"/bin",
	} {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		parts = append(parts, p)
	}

	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		parts = append(parts, p)
	}

	os.Setenv("PATH", strings.Join(parts, ":"))
	return nil
}

func bundleModprobePath(bundleDir string) string {
	p := filepath.Join(bundleDir, "bin", "modprobe")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}
