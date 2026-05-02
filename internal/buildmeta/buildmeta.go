// Package buildmeta holds compile-time build metadata (flavor, etc.).
package buildmeta

import "strings"

// Flavor is set at link time: "lite" (docker/podman only) or "full" (embedded containerd / iofog engine).
var Flavor = "lite"

const (
	FlavorLite = "lite"
	FlavorFull = "full"
)

// IsFull reports whether this binary is the full (embedded containerd) build.
func IsFull() bool {
	return strings.EqualFold(Flavor, FlavorFull)
}

// IsLite reports whether this binary is the lite (external engine only) build.
func IsLite() bool {
	return strings.EqualFold(Flavor, FlavorLite)
}

// AllowedEngines returns containerEngine values valid for this binary build.
func AllowedEngines() []string {
	if IsFull() {
		return []string{"iofog"}
	}
	return []string{"docker", "podman"}
}

// AllowedEnginesCSV returns AllowedEngines as a comma-separated string for CLI output.
func AllowedEnginesCSV() string {
	a := AllowedEngines()
	return strings.Join(a, ",")
}
