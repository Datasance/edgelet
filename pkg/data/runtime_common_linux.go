//go:build linux

package data

import (
	"fmt"
	"path/filepath"

	"github.com/datasance/edgelet/pkg/datadir"
	"github.com/datasance/edgelet/pkg/dataverify"
)

const fatRuntimeName = dataverify.FatRuntimeName

// RuntimeBinary returns the path to the extracted fat edgelet runtime ELF.
// Resolution order matches k3s authoritative data/<embed-hash>/ then data/current fallback.
func RuntimeBinary() (string, error) {
	if dir := ExtractDir(); dir != "" {
		if path, err := fatRuntimeInDir(dir); err == nil {
			return path, nil
		}
	}

	root, err := datadir.BundleRoot("")
	if err != nil {
		return "", fmt.Errorf("resolve fat runtime: %w", err)
	}
	if hash := EmbeddedBundleHash(); hash != "" {
		if path, err := fatRuntimeInDir(filepath.Join(root, hash)); err == nil {
			return path, nil
		}
	}
	current := filepath.Join(root, "current", "bin", fatRuntimeName)
	return fatRuntimeAt(current)
}

func fatRuntimeInDir(dir string) (string, error) {
	return fatRuntimeAt(filepath.Join(dir, "bin", fatRuntimeName))
}

func fatRuntimeAt(path string) (string, error) {
	if err := dataverify.VerifyFatRuntime(path); err != nil {
		return "", err
	}
	return path, nil
}
