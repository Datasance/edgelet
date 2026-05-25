//go:build !linux || !cgo

package data

import (
	"fmt"
)

// Asset is unavailable on lite and non-linux builds.
func Asset(name string) ([]byte, error) {
	return nil, fmt.Errorf("embedded data bundle not available in this build")
}

// AssetNames is empty on lite and non-linux builds.
func AssetNames() []string {
	return nil
}
