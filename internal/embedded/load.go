//go:build (linux && amd64) || (linux && arm64) || (linux && arm) || (linux && riscv64)

package embedded

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/utils/logging"
)

var loadLogger = logging.NewModuleLogger("Embedded")

// EnsureEmbeddedDependencies extracts all embedded binaries to disk,
// writes the CNI bridge config, loads required kernel modules, and enables
// IP forwarding. It is idempotent — existing files are not overwritten.
//
// Must be called before starting the containerd in-process service.
func EnsureEmbeddedDependencies() error {
	if err := loadContainerdComponents(); err != nil {
		return fmt.Errorf("failed to load containerd components: %w", err)
	}

	if err := loadCNIPlugins(); err != nil {
		return fmt.Errorf("failed to load CNI plugins: %w", err)
	}

	if err := loadCNIConfig(); err != nil {
		return fmt.Errorf("failed to load CNI config: %w", err)
	}

	if err := loadPauseImage(); err != nil {
		return fmt.Errorf("failed to load pause image: %w", err)
	}

	if err := loadKernelModules(); err != nil {
		// Non-fatal: some modules may already be loaded or built-in.
		loadLogger.Warnf("Failed to load kernel modules (non-fatal): %v", err)
	}

	return nil
}

// loadContainerdComponents extracts the containerd-shim-runc-v2 and crun
// binaries to disk, then
// symlinks them into /usr/local/bin so that containerd's runtime v2 task
// plugin can resolve the shim by name (e.g. "io.containerd.runc.v2" →
// containerd-shim-runc-v2) without requiring the bin dir to be in PATH.
func loadContainerdComponents() error {
	if err := os.MkdirAll(constants.EdgeletContainerdBinDir, 0755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	binaries := []struct {
		data    []byte
		dest    string
		name    string
		symlink bool // create a /usr/local/bin symlink for PATH resolution
	}{
		{ContainerdShimRuncBinary, filepath.Join(constants.EdgeletContainerdBinDir, "containerd-shim-runc-v2"), "containerd-shim-runc-v2", true},
		{CrunBinary, filepath.Join(constants.EdgeletContainerdBinDir, "crun"), "crun", true},
	}

	for _, b := range binaries {
		if err := extractBinary(b.data, b.dest); err != nil {
			return fmt.Errorf("extract %s: %w", b.name, err)
		}
		if b.symlink {
			symlinkShimToPath(b.dest, b.name)
		}
	}
	return nil
}

// symlinkShimToPath creates (or refreshes) a symlink at /usr/local/bin/<name>
// pointing at the extracted binary. This allows containerd's runtime v2 task
// plugin to resolve shim names (e.g. "containerd-shim-runc-v2") by searching
// PATH without the caller needing to modify the environment.
func symlinkShimToPath(src, name string) {
	const pathDir = "/usr/local/bin"
	link := filepath.Join(pathDir, name)
	// Always refresh the symlink — the source binary may have been updated.
	_ = os.Remove(link)
	if err := os.Symlink(src, link); err != nil {
		loadLogger.Warnf("Could not symlink %s to %s (non-fatal): %v", name, pathDir, err)
	} else {
		loadLogger.Debugf("Linked %s -> %s", link, src)
	}
}

// loadCNIPlugins extracts the CNI plugin binaries used by embedded networking.
func loadCNIPlugins() error {
	if err := os.MkdirAll(constants.EdgeletCNIPluginsDir, 0755); err != nil {
		return fmt.Errorf("create CNI plugins dir: %w", err)
	}

	plugins := []struct {
		data []byte
		name string
	}{
		{CNIPluginBridge, "bridge"},
		{CNIPluginHostLocal, "host-local"},
		{CNIPluginPortmap, "portmap"},
		{CNIPluginLoopback, "loopback"},
	}

	for _, p := range plugins {
		dest := filepath.Join(constants.EdgeletCNIPluginsDir, p.name)
		if err := extractBinary(p.data, dest); err != nil {
			return fmt.Errorf("extract CNI plugin %s: %w", p.name, err)
		}
	}
	return nil
}

// loadCNIConfig writes the canonical CNI conflist and symlinks it into
// the standard system CNI config directory.
func loadCNIConfig() error {
	for _, dir := range []string{
		constants.EdgeletCNIConfDir,
		constants.DefaultSystemCNIConfDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create CNI conf dir %s: %w", dir, err)
		}
	}
	if err := writeAndSymlinkCNIConfig(
		generateManagedCNIConfig(),
		constants.EdgeletCNIConfigFile,
		filepath.Join(constants.DefaultSystemCNIConfDir, constants.DefaultCNIConfigName),
	); err != nil {
		return err
	}
	loadLogger.Infof("Single-bridge CNI config written")
	return nil
}

func writeAndSymlinkCNIConfig(cfg map[string]any, targetPath, systemLink string) error {
	cniConfig, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal CNI config: %w", err)
	}
	if err := os.WriteFile(targetPath, cniConfig, 0644); err != nil {
		return fmt.Errorf("write CNI config file: %w", err)
	}
	if systemLink != "" {
		_ = os.Remove(systemLink)
		if err := os.Symlink(targetPath, systemLink); err != nil && !os.IsExist(err) {
			return fmt.Errorf("symlink CNI config %s: %w", systemLink, err)
		}
	}
	return nil
}

// loadPauseImage extracts the embedded pause (sandbox) image to disk.
// The image is imported into containerd at engine Init.
func loadPauseImage() error {
	if len(PauseImageTarGz) == 0 {
		return nil
	}
	if err := os.MkdirAll(constants.EdgeletContainerdImagesDir, 0755); err != nil {
		return fmt.Errorf("create images dir: %w", err)
	}
	dest := filepath.Join(constants.EdgeletContainerdImagesDir, "pause.tar.gz")
	if _, err := os.Stat(dest); err == nil {
		return nil // Already extracted (idempotent).
	}
	if err := os.WriteFile(dest, PauseImageTarGz, 0644); err != nil {
		return fmt.Errorf("write pause image: %w", err)
	}
	loadLogger.Debugf("Pause image extracted to %s", dest)
	return nil
}

// loadKernelModules loads the kernel modules required by containerd and the
// iofog bridge network. Failures are logged but not fatal — modules may already
// be loaded or compiled into the kernel.
func loadKernelModules() error {
	modules := []string{
		"overlay",
		"br_netfilter",
		"ip_tables",
		"iptable_filter",
		"iptable_nat",
		"nf_conntrack",
	}

	var firstErr error
	for _, mod := range modules {
		if err := exec.Command("modprobe", mod).Run(); err != nil {
			loadLogger.Debugf("modprobe %s: %v", mod, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("modprobe %s: %w", mod, err)
			}
		}
	}

	// Enable IP forwarding unconditionally.
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		loadLogger.Debugf("Failed to enable IP forwarding: %v", err)
	}

	return firstErr
}

// extractBinary writes data to destFile with 0755 permissions.
// It avoids writing to an active executable path directly (ETXTBSY risk) by
// writing a temp file in the same directory then atomically renaming.
func extractBinary(data []byte, destFile string) error {
	// Skip rewrite when destination already has exact content.
	if existing, err := os.ReadFile(destFile); err == nil {
		if bytes.Equal(existing, data) {
			loadLogger.Debugf("Binary already up-to-date, skipping rewrite: %s", destFile)
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read existing %s: %w", destFile, err)
	}

	tmpFile := destFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0755); err != nil { // #nosec G306 -- binaries need execute permission
		return fmt.Errorf("write temp %s: %w", tmpFile, err)
	}

	if err := os.Rename(tmpFile, destFile); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("replace %s with temp binary: %w", destFile, err)
	}
	return nil
}
