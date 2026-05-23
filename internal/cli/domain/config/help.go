package config

import "strings"

// CommandLong returns the config command introduction shown before Cobra flags.
func CommandLong() string {
	return strings.TrimSpace(`Update agent configuration via flags.

Each setting has a long --kebab-case flag and a short --alias flag (see flag help below).
Use config cert to install a controller CA certificate, or config switch to change profile.`)
}

// CommandExamples returns config command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`# Long flags
  iofog-agent config --controller-url http://localhost:51121/api/v3
  iofog-agent config --change-frequency-seconds 10 --status-frequency-seconds 10
  iofog-agent config --disk-limit-gib 20 --memory-limit-mib 512

# Short alias flags
  iofog-agent config --a http://localhost:51121/api/v3 --cf 10 --sf 10

# Install controller CA certificate (base64-encoded PEM string)
  iofog-agent config cert <base64-encoded-cert-string>

# Switch configuration profile
  iofog-agent config switch prod

# Structured output (global flags before subcommand)
  iofog-agent -o json config --cf 10`)
}

// CertCommandLong returns help for config cert.
func CertCommandLong() string {
	return strings.TrimSpace(`Install the controller CA certificate from a base64-encoded PEM string.

The argument must be the certificate contents encoded as base64 (not a file path).
Use config --controller-cert for setting a local PEM file path instead.`)
}
