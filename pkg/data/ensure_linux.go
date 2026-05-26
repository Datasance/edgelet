//go:build linux && cgo

package data

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/datasance/edgelet/pkg/datadir"
	"github.com/datasance/edgelet/pkg/dataverify"
	"github.com/datasance/edgelet/pkg/flock"
	"github.com/datasance/edgelet/pkg/untar"
)

var (
	dataLogger = logging.NewModuleLogger("Data")

	extractDir   string
	extractDirMu sync.RWMutex
)

// ExtractDir returns the directory containing extracted bundle content.
func ExtractDir() string {
	extractDirMu.RLock()
	defer extractDirMu.RUnlock()
	return extractDir
}

// EnsureExtracted unpacks the embedded zstd bundle (if needed), verifies bin/,
// installs runtime auxiliaries to stable paths, and prepares CNI + pause image.
func EnsureExtracted() error {
	dir, err := extract("")
	if err != nil {
		return err
	}
	if err := installFromBundle(dir); err != nil {
		return err
	}
	if err := loadKernelModules(); err != nil {
		dataLogger.Warnf("Failed to load kernel modules (non-fatal): %v", err)
	}
	return nil
}

func getAssetAndDir(dataDir string) (string, string, error) {
	names := AssetNames()
	if len(names) == 0 {
		return "", "", fmt.Errorf("no embedded data bundle found (run scripts/package-data before building full profile)")
	}
	asset := names[len(names)-1]
	root, err := datadir.BundleRoot(dataDir)
	if err != nil {
		return "", "", err
	}
	hash := strings.SplitN(filepath.Base(asset), ".", 2)[0]
	dir := filepath.Join(root, hash)
	return asset, dir, nil
}

func extract(dataDir string) (string, error) {
	asset, dir, err := getAssetAndDir(dataDir)
	if err != nil {
		return "", err
	}

	if isBundleReady(dir) {
		setExtractDir(dir)
		return dir, nil
	}

	root, err := datadir.BundleRoot(dataDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("mkdir data root: %w", err)
	}

	lockFile := filepath.Join(root, ".lock")
	dataLogger.Infof("Acquiring lock file %s", lockFile)
	lock, err := flock.Acquire(lockFile)
	if err != nil {
		return "", fmt.Errorf("acquire extract lock: %w", err)
	}
	defer flock.Release(lock)

	if isBundleReady(dir) {
		setExtractDir(dir)
		return dir, nil
	}

	dataLogger.Infof("Preparing data dir %s", dir)

	content, err := Asset(asset)
	if err != nil {
		return "", fmt.Errorf("load embedded asset: %w", err)
	}

	tempDest := dir + "-tmp"
	_ = os.RemoveAll(tempDest)
	defer os.RemoveAll(tempDest)

	if err := untar.Untar(bytes.NewReader(content), tempDest); err != nil {
		return "", fmt.Errorf("untar bundle: %w", err)
	}
	if err := dataverify.Verify(filepath.Join(tempDest, "bin")); err != nil {
		return "", fmt.Errorf("verify extracted bin: %w", err)
	}

	currentSymlink := filepath.Join(root, "current")
	previousSymlink := filepath.Join(root, "previous")
	if _, err := os.Lstat(currentSymlink); err == nil {
		if err := os.Rename(currentSymlink, previousSymlink); err != nil {
			return "", fmt.Errorf("rotate current symlink: %w", err)
		}
	}
	if err := os.Symlink(dir, currentSymlink); err != nil {
		return "", fmt.Errorf("create current symlink: %w", err)
	}

	if err := os.Rename(tempDest, dir); err != nil {
		return "", fmt.Errorf("rename extracted bundle: %w", err)
	}

	if err := setupStableCNIDir(root, dir); err != nil {
		return "", err
	}

	setExtractDir(dir)
	return dir, nil
}

func isBundleReady(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "bin", "containerd-shim-runc-v2"))
	return err == nil
}

func setExtractDir(dir string) {
	extractDirMu.Lock()
	extractDir = dir
	extractDirMu.Unlock()
}

func setupStableCNIDir(root, dir string) error {
	cniPath := filepath.Join(root, "cni")
	cniBin := filepath.Join(dir, "bin", "cni")
	if err := os.MkdirAll(cniPath, 0755); err != nil {
		return fmt.Errorf("mkdir stable cni dir: %w", err)
	}
	_ = os.Remove(filepath.Join(cniPath, "cni"))
	if err := os.Symlink(cniBin, filepath.Join(cniPath, "cni")); err != nil {
		return fmt.Errorf("symlink cni multicall: %w", err)
	}

	ents, err := os.ReadDir(filepath.Join(dir, "bin"))
	if err != nil {
		return fmt.Errorf("read extracted bin: %w", err)
	}
	for _, ent := range ents {
		info, err := ent.Info()
		if err != nil || info.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(filepath.Join(dir, "bin", ent.Name()))
		if err != nil || target != "cni" {
			continue
		}
		src := filepath.Join(cniPath, ent.Name())
		if info, err := os.Lstat(src); err == nil && info.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		_ = os.Remove(src)
		if err := os.Symlink(cniBin, src); err != nil {
			return fmt.Errorf("symlink cni plugin %s: %w", ent.Name(), err)
		}
	}
	return nil
}

func installFromBundle(dir string) error {
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(constants.EdgeletContainerdBinDir, 0755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	binaries := []struct {
		name    string
		symlink bool
	}{
		{"containerd-shim-runc-v2", true},
		{"crun", true},
	}
	for _, b := range binaries {
		src := filepath.Join(binDir, b.name)
		dest := filepath.Join(constants.EdgeletContainerdBinDir, b.name)
		if err := installBinary(src, dest); err != nil {
			return fmt.Errorf("install %s: %w", b.name, err)
		}
		if b.symlink {
			symlinkShimToPath(dest, b.name)
		}
	}

	if err := installCNIPlugins(binDir); err != nil {
		return err
	}
	if err := loadCNIConfig(); err != nil {
		return err
	}
	return installPauseImage(dir)
}

func installCNIPlugins(binDir string) error {
	if err := os.MkdirAll(constants.EdgeletCNIPluginsDir, 0755); err != nil {
		return fmt.Errorf("create CNI plugins dir: %w", err)
	}
	plugins := []string{"bridge", "host-local", "portmap", "loopback"}
	for _, name := range plugins {
		src := filepath.Join(binDir, name)
		dest := filepath.Join(constants.EdgeletCNIPluginsDir, name)
		if err := installPluginLink(src, dest); err != nil {
			return fmt.Errorf("install CNI plugin %s: %w", name, err)
		}
	}
	return nil
}

func installPluginLink(src, dest string) error {
	_ = os.Remove(dest)
	return os.Symlink(src, dest)
}

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
	dataLogger.Infof("Single-bridge CNI config written")
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

func installPauseImage(dir string) error {
	src := filepath.Join(dir, "images", "pause.tar.gz")
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat pause image: %w", err)
	}
	if err := os.MkdirAll(constants.EdgeletContainerdImagesDir, 0755); err != nil {
		return fmt.Errorf("create images dir: %w", err)
	}
	dest := filepath.Join(constants.EdgeletContainerdImagesDir, "pause.tar.gz")
	return copyFileIfChanged(src, dest, 0644)
}

func installBinary(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	return extractBinary(data, dest)
}

func symlinkShimToPath(src, name string) {
	const pathDir = "/usr/local/bin"
	link := filepath.Join(pathDir, name)
	_ = os.Remove(link)
	if err := os.Symlink(src, link); err != nil {
		dataLogger.Warnf("Could not symlink %s to %s (non-fatal): %v", name, pathDir, err)
	} else {
		dataLogger.Debugf("Linked %s -> %s", link, src)
	}
}

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
			dataLogger.Debugf("modprobe %s: %v", mod, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("modprobe %s: %w", mod, err)
			}
		}
	}

	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		dataLogger.Debugf("Failed to enable IP forwarding: %v", err)
	}

	return firstErr
}

func extractBinary(data []byte, destFile string) error {
	if existing, err := os.ReadFile(destFile); err == nil {
		if bytes.Equal(existing, data) {
			dataLogger.Debugf("Binary already up-to-date, skipping rewrite: %s", destFile)
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

func copyFileIfChanged(src, dest string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if existing, err := os.ReadFile(dest); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write temp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", dest, err)
	}
	return nil
}
