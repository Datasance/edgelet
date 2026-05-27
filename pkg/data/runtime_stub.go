//go:build !linux

package data

import "fmt"

// EmbeddedBundleHash is empty on builds without an embedded bundle.
func EmbeddedBundleHash() string {
	return ""
}

// RuntimeBinary is unavailable on non-linux builds.
func RuntimeBinary() (string, error) {
	return "", fmt.Errorf("embedded runtime not available on this platform")
}
