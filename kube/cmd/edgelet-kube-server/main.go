//go:build linux && cgo

// Package main is the fat runtime.
// server starts containerd, imports images, and does not open the API port.
// The containerd child serves containerd and returns.
// kubectl, ctr, and crictl are recognized from argv0.
package main

/*
static void edgeletKubeRuntime(void) {}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/eclipse-iofog/edgelet-kube/internal/boot"
	"github.com/eclipse-iofog/edgelet-kube/internal/bundle"
	"github.com/eclipse-iofog/edgelet-kube/internal/config"
	"github.com/eclipse-iofog/edgelet-kube/internal/containerd"
	"github.com/eclipse-iofog/edgelet-kube/internal/datastore"
	"github.com/eclipse-iofog/edgelet-kube/internal/dispatch"
)

const readyLine = "edgelet-kube runtime"

func main() {
	// Keep cgo linked into the fat runtime.
	C.edgeletKubeRuntime()
	if handled, err := containerd.MaybeRunChild(os.Args); handled {
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "edgelet-kube: %v\n", err)
			os.Exit(1)
		}
		return
	}
	name, ok := commandName(os.Args)
	if !ok {
		_, _ = fmt.Fprintf(os.Stderr, "edgelet-kube: %v\n", dispatch.ErrUsage)
		os.Exit(2)
	}
	if containerd.WantsClient(name, os.Args) {
		var err error
		switch name {
		case "ctr":
			err = containerd.RunCtr(os.Args[1:])
		case "crictl":
			err = containerd.RunCrictl(os.Args[1:])
		}
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "edgelet-kube: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if name == bundle.ServerCommand {
		if err := serve(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "edgelet-kube: %v\n", err)
			os.Exit(1)
		}
		return
	}
	_, _ = fmt.Printf("%s %s\n", readyLine, name)
}

// serve starts containerd, imports images, and opens the datastore.
// It does not open the API port. Without permission to create the runtime
// directories, it returns after the ready line so the process does not open
// that port.
func serve() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	loaded, err := config.Load(os.Args, os.LookupEnv)
	if err != nil {
		return err
	}
	var store *datastore.Store
	proc, err := boot.Start(ctx, boot.Config{
		RegistryFile:           loaded.PrivateRegistry,
		DisableDefaultEndpoint: loaded.DisableDefaultRegistryEndpoint,
		PauseImage:             loaded.PauseImage,
		Registry:               loaded.SystemDefaultRegistry,
		Datastore: func(ctx context.Context) error {
			opened, err := datastore.Open(ctx, datastore.Config{
				DataDir:           loaded.DataDir,
				Endpoint:          loaded.DatastoreEndpoint,
				CAFile:            loaded.DatastoreCAFile,
				CertFile:          loaded.DatastoreCertFile,
				KeyFile:           loaded.DatastoreKeyFile,
				NATSCreds:         loaded.DatastoreNATSCreds,
				SecretsEncryption: loaded.SecretsEncryption,
			})
			if err != nil {
				return err
			}
			store = opened
			return nil
		},
	})
	if err != nil {
		if store != nil {
			store.Close()
		}
		if ctx.Err() != nil || errors.Is(err, os.ErrPermission) {
			_, _ = fmt.Printf("%s %s\n", readyLine, bundle.ServerCommand)
			return nil
		}
		return err
	}
	defer proc.Stop()
	defer store.Close()
	_, _ = fmt.Printf("%s %s\n", readyLine, bundle.ServerCommand)
	<-ctx.Done()
	return nil
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
