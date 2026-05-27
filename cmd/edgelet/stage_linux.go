//go:build linux && full && !cgo

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/datasance/edgelet/pkg/data"
)

func stageAndRunDaemon(args []string) error {
	if _, err := data.ExtractBundle(""); err != nil {
		return fmt.Errorf("extract embedded runtime: %w", err)
	}

	fatPath, err := data.RuntimeBinary()
	if err != nil {
		return fmt.Errorf("locate fat runtime: %w", err)
	}

	execArgs := append([]string{filepath.Base(fatPath)}, args[1:]...)
	if err := syscall.Exec(fatPath, execArgs, os.Environ()); err != nil {
		return fmt.Errorf("exec %s: %w", fatPath, err)
	}
	return nil
}
