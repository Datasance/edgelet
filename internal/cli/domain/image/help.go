package image

import "strings"

// CommandLong returns the image command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Local image operations via the agent container engine.

Subcommands: ls, pull, load, prune, rm.`)
}

// CommandExamples returns image command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`edgelet image ls -o json
  edgelet image pull docker.io/library/alpine:3.19
  edgelet image pull my.registry/app:1.0 -r 2 -p linux/amd64
  edgelet image load -f /path/to/image.tar
  edgelet image load -f /path/to/image.tar.gz
  edgelet image prune`)
}
