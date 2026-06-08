package output

import (
	"strings"
)

// VersionPayload is the combined CLI + daemon version object for structured output.
type VersionPayload struct {
	CLI    VersionCLI     `json:"cli"`
	Daemon map[string]any `json:"daemon,omitempty"`
}

// VersionCLI holds CLI build metadata.
type VersionCLI struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
	GitCommit string `json:"gitCommit"`
}

// BuildVersionPayload assembles version data from CLI build info and daemon API response.
func BuildVersionPayload(cliVersion, cliBuildTime, cliGitCommit string, daemon map[string]any, daemonErr error) VersionPayload {
	payload := VersionPayload{
		CLI: VersionCLI{
			Version:   cliVersion,
			BuildTime: cliBuildTime,
			GitCommit: cliGitCommit,
		},
	}
	if daemonErr == nil && len(daemon) > 0 {
		payload.Daemon = daemon
	}
	return payload
}

// FormatVersionHuman renders combined CLI + daemon version text.
func FormatVersionHuman(cliVersion, cliBuildTime, cliGitCommit string, daemon map[string]any, daemonErr error) string {
	var b strings.Builder
	_, _ = b.WriteString("cli.version: ")
	_, _ = b.WriteString(cliVersion)
	_ = b.WriteByte('\n')
	_, _ = b.WriteString("cli.buildTime: ")
	_, _ = b.WriteString(cliBuildTime)
	_ = b.WriteByte('\n')
	_, _ = b.WriteString("cli.gitCommit: ")
	_, _ = b.WriteString(cliGitCommit)
	_ = b.WriteByte('\n')

	if daemonErr != nil || len(daemon) == 0 {
		_, _ = b.WriteString("daemon: unavailable")
		return b.String()
	}
	_, _ = b.WriteString("daemon.version: ")
	_, _ = b.WriteString(MapValueAsString(daemon, "version"))
	_ = b.WriteByte('\n')
	_, _ = b.WriteString("daemon.buildTime: ")
	_, _ = b.WriteString(MapValueAsString(daemon, "buildTime"))
	_ = b.WriteByte('\n')
	_, _ = b.WriteString("daemon.gitCommit: ")
	_, _ = b.WriteString(MapValueAsString(daemon, "gitCommit"))
	_ = b.WriteByte('\n')

	allowed := MapValueAsString(daemon, "allowedContainerEngine")
	if allowed == "<unknown>" {
		allowed = MapValueAsString(daemon, "allowedEngines")
	}
	_, _ = b.WriteString("daemon.allowedContainerEngine: ")
	_, _ = b.WriteString(allowed)
	return b.String()
}
