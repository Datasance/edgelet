//go:build windows

package gps

import (
	"bufio"
	"errors"
	"os"
	"time"
)

// readLineWithTimeout is unsupported on Windows (Tier 2 desktop; serial poll is unix-only).
func (d *DeviceHandler) readLineWithTimeout(_ *os.File, _ *bufio.Reader, _ time.Duration) (string, error) {
	return "", errors.New("GPS serial device polling not supported on Windows")
}
