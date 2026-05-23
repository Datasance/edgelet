package output

import (
	"strings"
)

// VersionPayload is the combined CLI + daemon version object for structured output.
type VersionPayload struct {
	CLI    VersionCLI             `json:"cli"`
	Daemon map[string]interface{} `json:"daemon,omitempty"`
}

// VersionCLI holds CLI build metadata.
type VersionCLI struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
	GitCommit string `json:"gitCommit"`
}

// BuildVersionPayload assembles version data from CLI build info and daemon API response.
func BuildVersionPayload(cliVersion, cliBuildTime, cliGitCommit string, daemon map[string]interface{}, daemonErr error) VersionPayload {
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
func FormatVersionHuman(cliVersion, cliBuildTime, cliGitCommit string, daemon map[string]interface{}, daemonErr error) string {
	var b strings.Builder
	b.WriteString("cli.version: ")
	b.WriteString(cliVersion)
	b.WriteByte('\n')
	b.WriteString("cli.buildTime: ")
	b.WriteString(cliBuildTime)
	b.WriteByte('\n')
	b.WriteString("cli.gitCommit: ")
	b.WriteString(cliGitCommit)
	b.WriteByte('\n')

	if daemonErr != nil || len(daemon) == 0 {
		b.WriteString("daemon: unavailable")
		return b.String()
	}

	b.WriteString("daemon.version: ")
	b.WriteString(MapValueAsString(daemon, "version"))
	b.WriteByte('\n')
	b.WriteString("daemon.buildTime: ")
	b.WriteString(MapValueAsString(daemon, "buildTime"))
	b.WriteByte('\n')
	b.WriteString("daemon.gitCommit: ")
	b.WriteString(MapValueAsString(daemon, "gitCommit"))
	b.WriteByte('\n')
	b.WriteString("daemon.flavor: ")
	b.WriteString(MapValueAsString(daemon, "flavor"))
	b.WriteByte('\n')

	allowed := MapValueAsString(daemon, "allowedContainerEngine")
	if allowed == "<unknown>" {
		allowed = MapValueAsString(daemon, "allowedEngines")
	}
	b.WriteString("daemon.allowedContainerEngine: ")
	b.WriteString(allowed)
	return b.String()
}
