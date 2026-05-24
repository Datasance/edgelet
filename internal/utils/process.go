package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	pidFileName = "iofog-agentd.pid"
	// PIDFileName is the exported name for the PID file
	PIDFileName = pidFileName
)

// IsAnotherInstanceRunning checks if another instance of the daemon is running
func IsAnotherInstanceRunning() bool {
	pidFile := filepath.Join(VarRun, pidFileName)

	// Check if PID file exists
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return false
	}

	// Read PID from file
	pidBytes, err := os.ReadFile(pidFile) // #nosec G304 -- path is filepath.Join(constant dir, constant filename)
	if err != nil {
		return false
	}

	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		return false
	}

	// Check if process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Try to send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process doesn't exist, remove stale PID file
		if rerr := os.Remove(pidFile); rerr != nil && !os.IsNotExist(rerr) {
			// Cannot log here (may be called before logger init); best-effort remove
			_ = rerr
		}
		return false
	}

	return true
}

// WritePIDFile writes the current process ID to a file
func WritePIDFile() error {
	pidFile := filepath.Join(VarRun, pidFileName)

	// Ensure directory exists; 0755 is intentional: /var/run must be world-traversable
	mkErr := os.MkdirAll(VarRun, 0755) // #nosec G301
	if mkErr != nil {
		return fmt.Errorf("failed to create PID directory: %w", mkErr)
	}

	// Write PID; 0644 is intentional: PID files in /var/run are conventionally world-readable
	pid := os.Getpid()
	writeErr := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644) // #nosec G306
	if writeErr != nil {
		return fmt.Errorf("failed to write PID file: %w", writeErr)
	}

	logging.LogDebug("Process Manager", fmt.Sprintf("Wrote PID file: %s (PID: %d)", pidFile, pid))
	return nil
}

// RemovePIDFile removes the PID file
func RemovePIDFile() error {
	pidFile := filepath.Join(VarRun, pidFileName)
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}
