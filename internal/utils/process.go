package utils

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	pidFileName = "edgelet.pid"
	// PIDFileName is the exported name for the PID file
	PIDFileName = pidFileName
)

func removePIDFileBestEffort(pidFile string) {
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		_ = err
	}
}

func procCmdline(pid int) (string, error) {
	path := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := os.ReadFile(path) // #nosec G304 -- bounded /proc path
	if err != nil {
		return "", err
	}
	return string(bytes.ReplaceAll(data, []byte{0}, []byte{' '})), nil
}

func isEdgeletDaemonProcess(pid int) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	cmdline, err := procCmdline(pid)
	if err != nil {
		return false
	}
	cmdline = strings.ToLower(cmdline)
	return strings.Contains(cmdline, "edgelet") &&
		(strings.Contains(cmdline, "daemon") || strings.Contains(cmdline, "edgelet-server"))
}

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

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 {
		removePIDFileBestEffort(pidFile)
		return false
	}

	if pid == os.Getpid() {
		removePIDFileBestEffort(pidFile)
		return false
	}

	// Check if process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		removePIDFileBestEffort(pidFile)
		return false
	}

	// Try to send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		removePIDFileBestEffort(pidFile)
		return false
	}

	if !isEdgeletDaemonProcess(pid) {
		removePIDFileBestEffort(pidFile)
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
