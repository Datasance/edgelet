//go:build !linux

package main

import (
	"fmt"
	"os"

	"github.com/datasance/edgelet/internal/cli/cmd"
	edgeletcontainerdd "github.com/datasance/edgelet/pkg/containerd"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "edgelet panic: %v\n", r)
			os.Exit(1)
		}
	}()

	if handled, err := edgeletcontainerdd.MaybeRunChildProcess(os.Args); handled {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Embedded containerd child failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if cmd.ShouldRunCLI(os.Args) {
		os.Exit(cmd.Execute())
	}

	runDaemon()
}
