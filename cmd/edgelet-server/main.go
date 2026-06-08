//go:build linux && cgo

package main

import (
	"fmt"
	"os"

	edgeletcontainerdd "github.com/datasance/edgelet/pkg/containerd"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(os.Stderr, "edgelet panic: %v\n", r)
			os.Exit(1)
		}
	}()

	if handled, err := edgeletcontainerdd.MaybeRunChildProcess(os.Args); handled {
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Embedded containerd child failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "runtime-bootstrap" {
		runRuntimeBootstrap()
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] != "daemon" {
		_, _ = fmt.Fprintf(os.Stderr, "fat runtime only supports `edgelet daemon`, `edgelet runtime-bootstrap`, and --edgelet-containerd-child\n")
		os.Exit(1)
	}

	runDaemon()
}
