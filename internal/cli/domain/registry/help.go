package registry

import "strings"

// CommandLong returns the registry command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Manage local registry credentials used for image pulls and deploy manifests.

Subcommands: ls, inspect, rm.`)
}

// CommandExamples returns registry command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`edgelet registry ls -o json
  edgelet registry inspect <id>
  edgelet registry inspect <id> --password-plain
  edgelet registry rm <id>`)
}
