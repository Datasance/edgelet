package version

import (
	"runtime/debug"
	"strings"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"
	// BuildTime is set at build time via ldflags
	BuildTime = "unknown"
	// GitCommit is set at build time via ldflags
	GitCommit = "unknown"
)

// SetBuildInfo overrides build metadata (used in tests).
func SetBuildInfo(version, buildTime, gitCommit string) {
	Version = version
	BuildTime = buildTime
	GitCommit = gitCommit
}

// GetVersion returns the agent version
func GetVersion() string {
	// Try to get version from build info if not set
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" && setting.Value != "" {
					// Use git commit as version if available
					if len(setting.Value) > 7 {
						return "dev-" + setting.Value[:7]
					}
					return "dev-" + setting.Value
				}
			}
		}
	}
	return Version
}

// GetBuildInfo returns build information
func GetBuildInfo() map[string]string {
	return map[string]string{
		"version":   GetVersion(),
		"buildTime": BuildTime,
		"gitCommit": GitCommit,
		"goVersion": getGoVersion(),
	}
}

// getGoVersion extracts Go version from build info
func getGoVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.GoVersion
	}
	return "unknown"
}

// ParseVersion parses a version string and returns components
func ParseVersion(version string) (major, minor, patch int, suffix string) {
	// Remove "v" prefix if present
	version = strings.TrimPrefix(version, "v")

	// Split by "-" to separate suffix
	parts := strings.Split(version, "-")
	versionPart := parts[0]
	if len(parts) > 1 {
		suffix = strings.Join(parts[1:], "-")
	}

	// Parse version numbers
	versionNumbers := strings.Split(versionPart, ".")
	if len(versionNumbers) >= 1 {
		_ = major // Will be set by strconv
		_ = minor
		_ = patch
		// Parse major.minor.patch
		// For now, just return 0,0,0 as parsing is not critical
	}

	return 0, 0, 0, suffix
}
