package engine

import "strings"

// SupportsInPlaceRestart reports whether StopContainer leaves a container object
// that StartContainer can bring back without Remove+Create.
func SupportsInPlaceRestart(engineName string) bool {
	switch strings.ToLower(strings.TrimSpace(engineName)) {
	case "docker", "podman":
		return true
	default:
		return false
	}
}
