//go:build !linux || !full

package data

import "fmt"

// EmbeddedBundleHash is empty on builds without an embedded bundle.
func EmbeddedBundleHash() string {
	return ""
}

// RuntimeBinary is unavailable on lite and non-linux builds.
func RuntimeBinary() (string, error) {
	return "", fmt.Errorf("embedded runtime not available in this build")
}
