//go:build linux && cgo

// Package main is the fat runtime. server returns without opening the API port.
// kubectl, ctr, and crictl are recognized from argv0.
package main

/*
static void edgeletKubeRuntime(void) {}
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eclipse-iofog/edgelet-kube/internal/bundle"
	"github.com/eclipse-iofog/edgelet-kube/internal/dispatch"
)

const readyLine = "edgelet-kube runtime"

func main() {
	name, ok := commandName(os.Args)
	if !ok {
		_, _ = fmt.Fprintf(os.Stderr, "edgelet-kube: %v\n", dispatch.ErrUsage)
		os.Exit(2)
	}
	// Keep cgo linked into the fat runtime.
	C.edgeletKubeRuntime()
	_, _ = fmt.Printf("%s %s\n", readyLine, name)
}

func commandName(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	base := filepath.Base(args[0])
	if bundle.Client(base) {
		return base, true
	}
	flag := "--" + bundle.DataDirFlag
	for i := 1; i < len(args); i++ {
		switch {
		case args[i] == flag:
			i++
		case strings.HasPrefix(args[i], flag+"="):
		case args[i] == bundle.ServerCommand:
			return bundle.ServerCommand, true
		}
	}
	return "", false
}
