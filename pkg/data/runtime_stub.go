//go:build !linux

package data

import (
	"errors"
)

// EmbeddedBundleHash is empty on builds without an embedded bundle.
func EmbeddedBundleHash() string {
	return ""
}

// RuntimeBinary is unavailable on non-linux builds.
func RuntimeBinary() (string, error) {
	return "", errors.New("embedded runtime not available on this platform")
}
