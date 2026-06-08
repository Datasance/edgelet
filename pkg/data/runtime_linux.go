//go:build linux && !cgo

package data

import (
	"path/filepath"
	"strings"
)

// EmbeddedBundleHash returns the SHA256 prefix of the embedded zstd bundle, or empty.
func EmbeddedBundleHash() string {
	names := AssetNames()
	if len(names) == 0 {
		return ""
	}
	return strings.SplitN(filepath.Base(names[len(names)-1]), ".", 2)[0]
}
