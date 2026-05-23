package auth

import "strings"

// CommandLong returns the auth command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Inspect and manage LocalAPI authentication tokens.

Subcommands: whoami, tokens, revoke.`)
}

// CommandExamples returns auth command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`iofog-agent auth whoami -o json
  iofog-agent auth tokens -o json
  iofog-agent auth revoke <jti>`)
}
