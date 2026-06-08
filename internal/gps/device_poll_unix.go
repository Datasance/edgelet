//go:build unix

package gps

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// readLineWithTimeout reads a line from reader with timeout (unix poll).
func (d *DeviceHandler) readLineWithTimeout(file *os.File, reader *bufio.Reader, timeout time.Duration) (string, error) {
	if file == nil {
		return "", errors.New("nil device file")
	}

	pollFds := []unix.PollFd{{
		Fd:     int32(file.Fd()), // #nosec G115 -- serial device FD from os.File fits int32 on supported platforms
		Events: unix.POLLIN | unix.POLLPRI,
	}}
	pollTimeoutMs := int(timeout.Milliseconds())
	if pollTimeoutMs < 0 {
		pollTimeoutMs = -1
	}

	n, err := unix.Poll(pollFds, pollTimeoutMs)
	if err != nil {
		return "", fmt.Errorf("poll failed: %w", err)
	}
	if n == 0 {
		return "", errors.New("read timeout")
	}
	if pollFds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
		return "", errors.New("device poll error")
	}
	if pollFds[0].Revents&(unix.POLLIN|unix.POLLPRI) == 0 {
		return "", errors.New("device not readable")
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
