package datadir

import (
	"path/filepath"

	"github.com/datasance/edgelet/internal/constants"
)

// DefaultDataDir is the root for extracted zstd bundle data.
const DefaultDataDir = constants.EdgeletDataDir

// Resolve returns an absolute data directory path.
func Resolve(dataDir string) (string, error) {
	if dataDir == "" {
		dataDir = DefaultDataDir
	}
	return filepath.Abs(dataDir)
}

// BundleRoot returns .../data under the resolved data dir.
func BundleRoot(dataDir string) (string, error) {
	root, err := Resolve(dataDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "data"), nil
}
