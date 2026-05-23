package image

import "strings"

// CommandLong returns the image command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Local image operations via the agent container engine.

Subcommands: ls, pull, load, prune, rm.`)
}

// CommandExamples returns image command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`iofog-agent image ls -o json
  iofog-agent image pull docker.io/library/alpine:3.19
  iofog-agent image pull my.registry/app:1.0 -r 2 -p linux/amd64
  iofog-agent image load -f /path/to/image.tar
  iofog-agent image prune`)
}
