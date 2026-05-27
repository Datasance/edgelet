//go:build !linux || !full

package data

// EnsureExtracted is a no-op on lite builds and non-Linux platforms.
func EnsureExtracted() error {
	return nil
}

// ExtractDir returns empty on platforms without an embedded bundle.
func ExtractDir() string {
	return ""
}
