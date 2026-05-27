//go:build linux && full

package data

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/datasance/edgelet/pkg/datadir"
	"github.com/datasance/edgelet/pkg/dataverify"
)

const fatRuntimeName = dataverify.FatRuntimeName

// EmbeddedBundleHash returns the SHA256 prefix of the embedded zstd bundle, or empty.
func EmbeddedBundleHash() string {
	names := AssetNames()
	if len(names) == 0 {
		return ""
	}
	return strings.SplitN(filepath.Base(names[len(names)-1]), ".", 2)[0]
}

// RuntimeBinary returns the path to the extracted fat edgelet runtime ELF.
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
