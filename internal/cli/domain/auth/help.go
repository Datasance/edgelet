package auth

import "strings"

// CommandLong returns the auth command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Inspect and manage LocalAPI authentication tokens.

Subcommands: whoami, tokens, revoke.`)
}

// CommandExamples returns auth command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`edgelet auth whoami -o json
  edgelet auth tokens -o json
  edgelet auth revoke <jti>`)
}
