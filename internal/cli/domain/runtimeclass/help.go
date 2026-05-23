package runtimeclass

import "strings"

// CommandLong returns the runtimeclass command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Manage runtime class definitions for local microservice deploy manifests.

Subcommands: ls, inspect, rm.`)
}

// CommandExamples returns runtimeclass command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`iofog-agent runtimeclass ls -o json
  iofog-agent runtimeclass inspect <name>
  iofog-agent runtimeclass rm <name>`)
}
