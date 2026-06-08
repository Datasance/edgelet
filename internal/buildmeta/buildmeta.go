// Package buildmeta holds compile-time build metadata (platform capabilities).
package buildmeta

import "strings"

var testOverrideHasEmbeddedEngine *bool

// SetHasEmbeddedEngineForTest overrides HasEmbeddedEngine for tests. Pass nil to clear.
func SetHasEmbeddedEngineForTest(v *bool) {
	testOverrideHasEmbeddedEngine = v
}

// HasEmbeddedEngine reports whether this binary can run the embedded edgelet/containerd engine.
func HasEmbeddedEngine() bool {
	if testOverrideHasEmbeddedEngine != nil {
		return *testOverrideHasEmbeddedEngine
	}
	return platformHasEmbeddedEngine()
}

// AllowedEngines returns containerEngine values valid for this binary build.
func AllowedEngines() []string {
	if HasEmbeddedEngine() {
		return []string{"edgelet", "docker", "podman"}
	}
	return []string{"docker", "podman"}
}

// AllowedEnginesCSV returns AllowedEngines as a comma-separated string for CLI output.
func AllowedEnginesCSV() string {
	return strings.Join(AllowedEngines(), ",")
}
