//go:build linux && full

package main

import (
	"fmt"
	"os"

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

	if len(os.Args) > 1 && os.Args[1] != "daemon" {
		fmt.Fprintf(os.Stderr, "fat runtime only supports `edgelet daemon` and --edgelet-containerd-child\n")
		os.Exit(1)
	}

	runDaemon()
}
