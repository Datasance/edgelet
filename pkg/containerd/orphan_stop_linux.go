//go:build linux

package containerd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const orphanStopGrace = 5 * time.Second

func stopOrphanedEmbeddedContainerdFromProc() error {
	pids, err := findContainerdChildPIDs()
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		return nil
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	deadline := time.Now().Add(orphanStopGrace)
	for time.Now().Before(deadline) {
		remaining, err := findContainerdChildPIDs()
		if err != nil {
			return err
		}
		if len(remaining) == 0 {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

func findContainerdChildPIDs() ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}
	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil || pid <= 1 {
			continue
		}
		cmdline, readErr := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if readErr != nil {
			continue
		}
		text := strings.ReplaceAll(string(cmdline), "\x00", " ")
		if strings.Contains(text, containerdChildArg) {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
