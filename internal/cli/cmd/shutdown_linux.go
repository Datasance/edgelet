//go:build linux

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/cli/domain/system"
	"github.com/spf13/cobra"
)

const defaultShutdownGraceSec = 90

func newShutdownCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "shutdown",
		Short: "Control-plane stop for init systems",
		Long:  "Gracefully stops the edgelet daemon. Used by systemd ExecStop and tier-1/2 init scripts. Drain policy is coordinated in Plan 11.",
		Args:  cobra.NoArgs,
		RunE:  runShutdown,
	}
}

func runShutdown(cmd *cobra.Command, args []string) error {
	grace := time.Duration(defaultShutdownGraceSec) * time.Second
	if appCtx != nil && appCtx.Client != nil && appCtx.Client.IsDaemonRunning() {
		_, err := system.Stop(appCtx.Client)
		if err == nil {
			if waitErr := waitForDaemonExit(grace); waitErr == nil {
				return nil
			}
		}
	}
	return signalShutdownFallback(grace)
}

func waitForDaemonExit(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if appCtx == nil || appCtx.Client == nil || !appCtx.Client.IsDaemonRunning() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("daemon still running after %s", timeout)
}

func signalShutdownFallback(timeout time.Duration) error {
	pids, err := findDaemonPIDs()
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		return nil
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remaining, _ := findDaemonPIDs()
		if len(remaining) == 0 {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

func findDaemonPIDs() ([]int, error) {
	if pid, ok := readPIDFile("/run/edgelet.pid"); ok {
		return []int{pid}, nil
	}
	if pid, ok := readPIDFile("/run/edgelet/edgelet.pid"); ok {
		return []int{pid}, nil
	}
	out, err := exec.Command("pgrep", "-f", "[e]dgelet daemon").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, convErr := strconv.Atoi(line)
		if convErr != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func readPIDFile(path string) (int, bool) {
	var (
		data []byte
		err  error
	)
	switch filepath.Clean(path) {
	case "/run/edgelet.pid":
		data, err = readFileUnderRoot("/run", "edgelet.pid")
	case "/run/edgelet/edgelet.pid":
		data, err = readFileUnderRoot("/run/edgelet", "edgelet.pid")
	default:
		return 0, false
	}
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		return 0, false
	}
	return pid, true
}

func readFileUnderRoot(rootDir, name string) ([]byte, error) {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid relative path %q", name)
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(name)
}
