//go:build (linux && amd64) || (linux && arm64) || (linux && arm) || (linux && riscv64)

package embedded

import (
	"bytes"
	"debug/elf"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/eclipse-iofog/agent/internal/constants"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
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

// loadContainerdComponents extracts the containerd-shim-runc-v2,
// containerd-shim-spin (when non-nil), and runc binaries to disk, then
// symlinks them into /usr/local/bin so that containerd's runtime v2 task
// plugin can resolve the shim by name (e.g. "io.containerd.runc.v2" →
// containerd-shim-runc-v2) without requiring the bin dir to be in PATH.
func loadContainerdComponents() error {
	if err := os.MkdirAll(constants.IofogContainerdBinDir, 0755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	binaries := []struct {
		data    []byte
		dest    string
		name    string
		symlink bool // create a /usr/local/bin symlink for PATH resolution
	}{
		{ContainerdShimRuncBinary, filepath.Join(constants.IofogContainerdBinDir, "containerd-shim-runc-v2"), "containerd-shim-runc-v2", true},
		{RuncBinary, filepath.Join(constants.IofogContainerdBinDir, "runc"), "runc", true},
	}

	// containerd-shim-spin is nil on riscv64.
	if len(ContainerdShimSpinBinary) > 0 {
		binaries = append(binaries, struct {
			data    []byte
			dest    string
			name    string
			symlink bool
		}{ContainerdShimSpinBinary, filepath.Join(constants.IofogContainerdBinDir, "containerd-shim-spin"), "containerd-shim-spin", true})
	}

	for _, b := range binaries {
		// Validate containerd-shim-spin architecture before extraction to catch cross-arch builds early
		if b.name == "containerd-shim-spin" {
			if err := validateBinaryArchitecture(b.data, runtime.GOARCH, b.name); err != nil {
				return fmt.Errorf("architecture mismatch for %s: %w (build for %s, running on %s — run 'make deps ARCH=%s' before building)",
					b.name, err, elfArchToGoArch(b.data), runtime.GOARCH, runtime.GOARCH)
			}
		}
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

// loadCNIPlugins extracts the four CNI plugin binaries.
func loadCNIPlugins() error {
	if err := os.MkdirAll(constants.IofogCNIPluginsDir, 0755); err != nil {
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
		dest := filepath.Join(constants.IofogCNIPluginsDir, p.name)
		if err := extractBinary(p.data, dest); err != nil {
			return fmt.Errorf("extract CNI plugin %s: %w", p.name, err)
		}
	}
	return nil
}

// loadCNIConfig writes managed/local CNI conflists and symlinks them into the
// standard system CNI config directory so containerd's CRI plugin finds both.
func loadCNIConfig() error {
	for _, dir := range []string{
		constants.IofogCNIConfDir,
		constants.IofogManagedCNIConfDir,
		constants.IofogLocalCNIConfDir,
		constants.DefaultSystemCNIConfDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create CNI conf dir %s: %w", dir, err)
		}
	}

	if err := writeAndSymlinkCNIConfig(
		generateManagedCNIConfig(),
		constants.IofogCNIConfigFile,
		filepath.Join(constants.DefaultSystemCNIConfDir, constants.IofogManagedCNIConfigName),
	); err != nil {
		return err
	}
	if err := writeAndSymlinkCNIConfig(
		generateLocalCNIConfig(),
		constants.IofogLocalCNIConfigFile,
		filepath.Join(constants.DefaultSystemCNIConfDir, constants.IofogLocalCNIConfigName),
	); err != nil {
		return err
	}
	loadLogger.Infof("Managed/local CNI bridge configs written and symlinked")
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
	_ = os.Remove(systemLink)
	if err := os.Symlink(targetPath, systemLink); err != nil && !os.IsExist(err) {
		return fmt.Errorf("symlink CNI config %s: %w", systemLink, err)
	}
	return nil
}

// loadPauseImage extracts the embedded pause (sandbox) image to disk.
// The image is imported into containerd at engine Init.
func loadPauseImage() error {
	if len(PauseImageTarGz) == 0 {
		return nil
	}
	if err := os.MkdirAll(constants.IofogContainerdImagesDir, 0755); err != nil {
		return fmt.Errorf("create images dir: %w", err)
	}
	dest := filepath.Join(constants.IofogContainerdImagesDir, "pause.tar.gz")
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
// Always overwrites existing files so the correct architecture binary is used
// after cross-compilation (e.g. make deps ARCH=arm64 && build-linux-arm64).
func extractBinary(data []byte, destFile string) error {
	if err := os.WriteFile(destFile, data, 0755); err != nil { // #nosec G306 -- binaries need execute permission
		return fmt.Errorf("write %s: %w", destFile, err)
	}
	return nil
}

// elfArchToGoArch returns the GOARCH string for an ELF binary, or "unknown" if unreadable.
func elfArchToGoArch(data []byte) string {
	f, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	switch f.Machine {
	case elf.EM_X86_64:
		return "amd64"
	case elf.EM_AARCH64:
		return "arm64"
	case elf.EM_386:
		return "386"
	case elf.EM_ARM:
		return "arm"
	case elf.EM_RISCV:
		return "riscv64"
	default:
		return fmt.Sprintf("elf-%d", f.Machine)
	}
}

// validateBinaryArchitecture checks that the ELF binary matches the expected GOARCH.
// Returns an error if there is a mismatch (e.g. amd64 binary on arm64 host → exec format error).
func validateBinaryArchitecture(data []byte, expectedGOARCH string, name string) error {
	actual := elfArchToGoArch(data)
	if actual == "unknown" {
		return nil // Skip validation if we can't read the binary
	}
	if actual != expectedGOARCH {
		return fmt.Errorf("binary is %s but host is %s", actual, expectedGOARCH)
	}
	return nil
}
