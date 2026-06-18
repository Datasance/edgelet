package docker

import (
	"errors"
	"fmt"
	"io"

	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/moby/moby/client"
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

	execResp, err := cli.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		Cmd:          command,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true,
	})
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

	execAttachResp, err := cli.ExecAttach(ctx, execID, client.ExecAttachOptions{
		TTY: true,
	})
	if err != nil {
		logging.LogError(execModuleName, "Error starting exec session", err)
		return fmt.Errorf("failed to start exec session: %w", err)
	}
	defer func() {
		execAttachResp.Close()
	}()

	done := make(chan error, 3)

	if stdin != nil {
		go func() {
			_, err := io.Copy(execAttachResp.Conn, stdin)
			done <- err
		}()
	}

	if stdout != nil {
		go func() {
			_, err := io.Copy(stdout, execAttachResp.Reader)
			done <- err
		}()
	}

	if stderr != nil && stdout == nil {
		go func() {
			_, err := io.Copy(stderr, execAttachResp.Reader)
			done <- err
		}()
	}

	err = <-done
	if err != nil && !errors.Is(err, io.EOF) {
		logging.LogError(execModuleName, "Error in exec session I/O", err)
		return err
	}

	logging.LogInfo(execModuleName, fmt.Sprintf("Exec session completed: %s", execID))
	return nil
}

// GetExecSessionStatus gets the status of an exec session
func (c *Client) GetExecSessionStatus(execID string) (*client.ExecInspectResult, error) {
	logging.LogDebug(execModuleName, fmt.Sprintf("Getting exec session status: %s", execID))

	cli := c.GetClient()
	if cli == nil {
		return nil, errors.New("docker client not initialized")
	}

	ctx := c.GetContext()

	execInspect, err := cli.ExecInspect(ctx, execID, client.ExecInspectOptions{})
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
	_, err := cli.ExecResize(c.GetContext(), execID, client.ExecResizeOptions{
		Width:  uint(cols),
		Height: uint(rows),
	})
	return err
}

// KillExecSession kills an exec session
func (c *Client) KillExecSession(execID string) error {
	logging.LogInfo(execModuleName, fmt.Sprintf("Killing exec session: %s", execID))

	cli := c.GetClient()
	if cli == nil {
		return errors.New("docker client not initialized")
	}

	ctx := c.GetContext()

	_, err := cli.ExecInspect(ctx, execID, client.ExecInspectOptions{})
	if err != nil {
		logging.LogError(execModuleName, "Error inspecting exec session for kill", err)
		return fmt.Errorf("failed to inspect exec session: %w", err)
	}

	logging.LogInfo(execModuleName, fmt.Sprintf("Exec session kill requested: %s", execID))
	return nil
}
