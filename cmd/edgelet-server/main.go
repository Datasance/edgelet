//go:build linux && cgo

package main

import (
	"fmt"
	"os"

	"github.com/datasance/edgelet/pkg/containerd"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(os.Stderr, "edgelet panic: %v\n", r)
			exitDaemon(1)
		}
	}()

	if handled, err := containerd.MaybeRunChildProcess(os.Args); handled {
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Embedded containerd child failed: %v\n", err)
			exitDaemon(1)
		}
		exitDaemon(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "runtime-bootstrap" {
		runRuntimeBootstrap()
		exitDaemon(0)
	}

	if len(os.Args) > 1 && os.Args[1] != "daemon" {
		_, _ = fmt.Fprint(os.Stderr, "fat runtime only supports `edgelet daemon`, `edgelet runtime-bootstrap`, and --edgelet-containerd-child\n")
		exitDaemon(1)
	}

	runDaemon()
}
