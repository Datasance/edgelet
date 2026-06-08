//go:build !linux

package main

import (
	"fmt"
	"os"

	"github.com/eclipse-iofog/edgelet/internal/cli/cmd"
	"github.com/eclipse-iofog/edgelet/pkg/containerd"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(os.Stderr, "edgelet panic: %v\n", r)
			os.Exit(1)
		}
	}()

	if handled, err := containerd.MaybeRunChildProcess(os.Args); handled {
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Embedded containerd child failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if cmd.ShouldRunCLI(os.Args) {
		os.Exit(cmd.Execute())
	}

	runDaemon()
}
