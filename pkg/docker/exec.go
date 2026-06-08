package docker

import (
	"errors"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

const (
	execModuleName = "Docker Exec"
)

// CreateExecSession creates an exec session in a container
func (c *Client) CreateExecSession(containerID string, command []string) (string, error) {
	logging.LogInfo(execModuleName, fmt.Sprintf("Creating exec session for container: %s, command: %v", containerID, command))

	cli := c.GetClient()
	if cli == nil {
		return "", errors.New("docker client not initialized")
	}

	ctx := c.GetContext()

	// Create exec configuration
	execConfig := types.ExecConfig{
		Cmd:          command,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Detach:       false,
	}

	// Create exec session
	execResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		logging.LogError(execModuleName, "Error creating exec session", err)
		return "", fmt.Errorf("failed to create exec session: %w", err)
	}

	execID := execResp.ID
	logging.LogInfo(execModuleName, fmt.Sprintf("Exec session created with ID: %s", execID))
	return execID, nil
}

// StartExecSession starts an exec session with I/O handling
func (c *Client) StartExecSession(execID string, stdin io.Reader, stdout, stderr io.Writer) error {
	logging.LogInfo(execModuleName, fmt.Sprintf("Starting exec session: %s", execID))

	cli := c.GetClient()
	if cli == nil {
		return errors.New("docker client not initialized")
	}

	ctx := c.GetContext()

	// Start exec session
	execStartCheck := types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	}

	// Attach to exec session
	execAttachResp, err := cli.ContainerExecAttach(ctx, execID, execStartCheck)
	if err != nil {
		logging.LogError(execModuleName, "Error starting exec session", err)
		return fmt.Errorf("failed to start exec session: %w", err)
	}
	defer func() {
		execAttachResp.Close()
	}()

	// Handle I/O in goroutines
	done := make(chan error, 3)

	// Copy stdin to exec session
	if stdin != nil {
		go func() {
			_, err := io.Copy(execAttachResp.Conn, stdin)
			done <- err
		}()
	}

	// Copy stdout from exec session
	if stdout != nil {
		go func() {
			_, err := io.Copy(stdout, execAttachResp.Reader)
			done <- err
		}()
	}

	// Copy stderr from exec session (same reader as stdout for TTY)
	if stderr != nil && stdout == nil {
		go func() {
			_, err := io.Copy(stderr, execAttachResp.Reader)
			done <- err
		}()
	}

	// Wait for completion
	err = <-done
	if err != nil && !errors.Is(err, io.EOF) {
		logging.LogError(execModuleName, "Error in exec session I/O", err)
		return err
	}

	logging.LogInfo(execModuleName, fmt.Sprintf("Exec session completed: %s", execID))
	return nil
}

// GetExecSessionStatus gets the status of an exec session
func (c *Client) GetExecSessionStatus(execID string) (*types.ContainerExecInspect, error) {
	logging.LogDebug(execModuleName, fmt.Sprintf("Getting exec session status: %s", execID))

	cli := c.GetClient()
	if cli == nil {
		return nil, errors.New("docker client not initialized")
	}

	ctx := c.GetContext()

	// Inspect exec session
	execInspect, err := cli.ContainerExecInspect(ctx, execID)
	if err != nil {
		logging.LogError(execModuleName, "Error getting exec session status", err)
		return nil, fmt.Errorf("failed to get exec session status: %w", err)
	}

	return &execInspect, nil
}

// GetExecSessionExitCode returns an exit code only for completed exec sessions.
func (c *Client) GetExecSessionExitCode(execID string) (int, error) {
	info, err := c.GetExecSessionStatus(execID)
	if err != nil {
		return 0, err
	}
	if info.Running {
		return 0, errors.New("exec session is still running")
	}
	return info.ExitCode, nil
}

// ResizeExecSession resizes a running TTY exec session.
func (c *Client) ResizeExecSession(execID string, cols, rows uint32) error {
	cli := c.GetClient()
	if cli == nil {
		return errors.New("docker client not initialized")
	}
	return cli.ContainerExecResize(c.GetContext(), execID, container.ResizeOptions{
		Width:  uint(cols),
		Height: uint(rows),
	})
}

// KillExecSession kills an exec session
func (c *Client) KillExecSession(execID string) error {
	logging.LogInfo(execModuleName, fmt.Sprintf("Killing exec session: %s", execID))

	cli := c.GetClient()
	if cli == nil {
		return errors.New("docker client not initialized")
	}

	ctx := c.GetContext()

	// Note: Docker API doesn't have a direct "kill exec" endpoint
	// We can only inspect the exec session to check if it's running
	// The exec session will terminate when the process exits
	// For force termination, we would need to send a signal to the process
	// This is typically handled by the exec session callback

	// Inspect to verify exec session exists
	_, err := cli.ContainerExecInspect(ctx, execID)
	if err != nil {
		logging.LogError(execModuleName, "Error inspecting exec session for kill", err)
		return fmt.Errorf("failed to inspect exec session: %w", err)
	}

	logging.LogInfo(execModuleName, fmt.Sprintf("Exec session kill requested: %s", execID))
	return nil
}
