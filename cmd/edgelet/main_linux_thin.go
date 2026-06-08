//go:build linux && !cgo

package main

import (
	"fmt"
	"os"

	"github.com/eclipse-iofog/edgelet/internal/cli/cmd"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(os.Stderr, "edgelet panic: %v\n", r)
			os.Exit(1)
		}
	}()

	if cmd.ShouldRunCLI(os.Args) {
		os.Exit(cmd.Execute())
	}

	if err := stageAndRunDaemon(os.Args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to start edgelet daemon: %v\n", err)
		os.Exit(1)
	}
}
