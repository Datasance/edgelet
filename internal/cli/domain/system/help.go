package system

import "strings"

// CommandLong returns the system command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Agent runtime and daemon operations.

Subcommands: status, info, version, reload, stop, logs, prune.`)
}

// StopCommandLong returns help for system stop.
func StopCommandLong() string {
	return strings.TrimSpace(`Gracefully stop the ioFog Agent daemon (iofog-agentd).

WARNING: Stopping the daemon disables LocalAPI until the daemon is started again
(iofog-agentd or systemctl start iofog-agentd).`)
}
