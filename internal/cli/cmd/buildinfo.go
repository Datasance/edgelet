package cmd

// Build metadata injected at link time via -ldflags (see Makefile LDFLAGS_CLI).
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// SetBuildInfo overrides build metadata (used in tests).
func SetBuildInfo(version, buildTime, gitCommit string) {
	Version = version
	BuildTime = buildTime
	GitCommit = gitCommit
}
