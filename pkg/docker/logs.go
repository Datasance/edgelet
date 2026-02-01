package docker

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
)

// TailConfig represents configuration for log tailing
type TailConfig struct {
	Follow bool
	Lines  int
	Since  string // ISO 8601 timestamp
	Until  string // ISO 8601 timestamp
}

// StreamType represents the stream type (stdout or stderr)
type StreamType int

const (
	STDOUT StreamType = iota
	STDERR
)

// LogTailHandler handles log lines from container logs
type LogTailHandler interface {
	OnLogLine(sessionID, microserviceUUID string, lineBytes []byte, streamType StreamType)
	OnComplete(sessionID string)
	OnError(sessionID string, err error)
}

// TailContainerLogs tails container logs using Docker API
func (c *Client) TailContainerLogs(containerID string, sessionID, microserviceUUID string, handler LogTailHandler, tailConfig *TailConfig) error {
	c.logger.Infof("Starting to tail container logs: containerId=%s, sessionId=%s", containerID, sessionID)

	cli := c.GetClient()
	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()

	// Parse tail config with defaults
	follow := true
	tailLines := 100
	if tailConfig != nil {
		follow = tailConfig.Follow
		if tailConfig.Lines > 0 {
			tailLines = tailConfig.Lines
		}
		// Validate tail lines (1-10000)
		if tailLines < 1 {
			tailLines = 100
		}
		if tailLines > 10000 {
			tailLines = 10000
		}
	}

	// Build log options
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: false,
	}

	// Set tail lines
	if tailLines > 0 && tailLines < 10000 {
		options.Tail = strconv.Itoa(tailLines)
	}

	// Parse since timestamp if provided
	if tailConfig != nil && tailConfig.Since != "" {
		sinceTime, err := parseISOTimestamp(tailConfig.Since)
		if err != nil {
			c.logger.Warnf("Invalid since timestamp format: %s - %v", tailConfig.Since, err)
		} else {
			options.Since = sinceTime.Format(time.RFC3339)
		}
	}

	// Parse until timestamp if provided
	if tailConfig != nil && tailConfig.Until != "" {
		untilTime, err := parseISOTimestamp(tailConfig.Until)
		if err != nil {
			c.logger.Warnf("Invalid until timestamp format: %s - %v", tailConfig.Until, err)
		} else {
			options.Until = untilTime.Format(time.RFC3339)
		}
	}

	// Get logs stream
	logsReader, err := cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		c.logger.Errorf("Error tailing container logs: containerId=%s - %v", containerID, err)
		if handler != nil {
			handler.OnError(sessionID, err)
		}
		return err
	}

	// Stream logs in a goroutine (matching Java: async execution)
	go func() {
		// Close logsReader when goroutine exits (matching Java: reader is closed when callback completes)
		defer func() {
			if logsReader != nil {
				logsReader.Close()
			}
			if handler != nil {
				handler.OnComplete(sessionID)
			}
		}()

		// Use stdcopy to demultiplex stdout and stderr
		stdoutReader, stdoutWriter := io.Pipe()
		stderrReader, stderrWriter := io.Pipe()

		// Demultiplex the log stream
		demuxDone := make(chan error, 1)
		go func() {
			_, err := stdcopy.StdCopy(stdoutWriter, stderrWriter, logsReader)
			stdoutWriter.Close()
			stderrWriter.Close()
			demuxDone <- err
		}()

		// Read stdout
		stdoutDone := make(chan error, 1)
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := stdoutReader.Read(buf)
				if n > 0 {
					lines := strings.Split(string(buf[:n]), "\n")
					for _, line := range lines {
						if line != "" {
							if handler != nil {
								handler.OnLogLine(sessionID, microserviceUUID, []byte(line+"\n"), STDOUT)
							}
						}
					}
				}
				if err == io.EOF {
					stdoutDone <- nil
					break
				}
				if err != nil {
					stdoutDone <- err
					break
				}
			}
		}()

		// Read stderr
		stderrDone := make(chan error, 1)
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := stderrReader.Read(buf)
				if n > 0 {
					lines := strings.Split(string(buf[:n]), "\n")
					for _, line := range lines {
						if line != "" {
							if handler != nil {
								handler.OnLogLine(sessionID, microserviceUUID, []byte(line+"\n"), STDERR)
							}
						}
					}
				}
				if err == io.EOF {
					stderrDone <- nil
					break
				}
				if err != nil {
					stderrDone <- err
					break
				}
			}
		}()

		// Wait for demux to complete or error
		select {
		case err := <-demuxDone:
			if err != nil && err != io.EOF {
				if handler != nil {
					handler.OnError(sessionID, err)
				}
			}
		case <-ctx.Done():
			// Context cancelled, stop tailing
		}

		// Wait for readers to finish
		<-stdoutDone
		<-stderrDone
	}()

	c.logger.Infof("Started tailing container logs: containerId=%s", containerID)
	return nil
}

// parseISOTimestamp parses an ISO 8601 timestamp string
func parseISOTimestamp(timestamp string) (time.Time, error) {
	// Try RFC3339 format first (most common)
	t, err := time.Parse(time.RFC3339, timestamp)
	if err == nil {
		return t, nil
	}

	// Try RFC3339Nano
	t, err = time.Parse(time.RFC3339Nano, timestamp)
	if err == nil {
		return t, nil
	}

	// Try Unix timestamp (seconds)
	if unixTime, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
		return time.Unix(unixTime, 0), nil
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", timestamp)
}
