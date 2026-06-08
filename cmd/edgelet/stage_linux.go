//go:build linux && !cgo

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/constants"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/pkg/containerd"
)

func stageAndRunDaemon(args []string) error {
	engine := constants.EngineEdgelet
	subcommand := ""
	if len(args) > 1 {
		subcommand = args[1]
	}
	if subcommand == "daemon" || subcommand == "runtime-bootstrap" {
		if err := config.LoadConfig(utils.ConfigYAMLPath); err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		engine = strings.ToLower(strings.TrimSpace(config.GetInstance().ContainerEngine))
	}

	if engine == constants.EngineDocker || engine == constants.EnginePodman {
		if err := containerd.StopOrphanedEmbeddedContainerd(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: stop orphaned embedded containerd: %v\n", err)
		}
	}

	fatPath, err := dataRuntimeBinary()
	if err != nil {
		if err := dataEnsureExtracted(); err != nil {
			return fmt.Errorf("prepare embedded runtime: %w", err)
		}
		fatPath, err = dataRuntimeBinary()
		if err != nil {
			return fmt.Errorf("locate fat runtime: %w", err)
		}
	} else if engine == constants.EngineEdgelet {
		// Idempotent: refresh CNI/aux paths and current symlink even when fat is already on disk.
		if err := dataEnsureExtracted(); err != nil {
			return fmt.Errorf("prepare embedded runtime: %w", err)
		}
		fatPath, err = dataRuntimeBinary()
		if err != nil {
			return fmt.Errorf("locate fat runtime: %w", err)
		}
	}

	if len(args) > 1 && subcommand == "daemon" && os.Getenv("EDGELET_DAEMON") == "container" {
		_ = utils.RemovePIDFile()
	}

	execArgs := append([]string{filepath.Base(fatPath)}, args[1:]...)
	if err := syscall.Exec(fatPath, execArgs, os.Environ()); err != nil {
		return fmt.Errorf("exec %s: %w", fatPath, err)
	}
	return nil
}
