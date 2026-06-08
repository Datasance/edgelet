package utils //nolint:revive // legacy package name

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

const (
	commandExecutorModuleName = "CommandExecutor"
)

// ExecuteCommand executes a shell command and returns the output
func ExecuteCommand(command string) (string, string, error) {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", command) // #nosec G204 -- intentional shell utility; callers supply trusted commands
	} else {
		// Use /bin/sh for Unix-like systems
		cmd = exec.Command("/bin/sh", "-c", command) // #nosec G204 -- intentional shell utility; callers supply trusted commands
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := strings.TrimSpace(stdout.String())
	errorOutput := strings.TrimSpace(stderr.String())

	if err != nil {
		logging.LogError(commandExecutorModuleName, "Command execution failed", err)
		return output, errorOutput, err
	}

	return output, errorOutput, nil
}

// ExecuteCommandSafe executes a command with timeout and safety checks
func ExecuteCommandSafe(command string, timeoutSeconds int) (string, error) {
	if timeoutSeconds <= 0 {
		// Default timeout of 30 seconds
		timeoutSeconds = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-Command", command) // #nosec G204 -- intentional shell utility; callers supply trusted commands
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command) // #nosec G204 -- intentional shell utility; callers supply trusted commands
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := strings.TrimSpace(stdout.String())

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logging.LogError(commandExecutorModuleName, "Command execution timed out", err)
			return output, fmt.Errorf("command timed out after %d seconds", timeoutSeconds)
		}
		logging.LogError(commandExecutorModuleName, "Command execution failed", err)
		return output, err
	}

	return output, nil
}
