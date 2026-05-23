package registry

import "strings"

// CommandLong returns the registry command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Manage local registry credentials used for image pulls and deploy manifests.

Subcommands: ls, inspect, rm.`)
}

// CommandExamples returns registry command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`iofog-agent registry ls -o json
  iofog-agent registry inspect <id>
  iofog-agent registry inspect <id> --password-plain
  iofog-agent registry rm <id>`)
}
