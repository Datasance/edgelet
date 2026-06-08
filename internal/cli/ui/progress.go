package ui

import (
	"fmt"
	"io"
	"strings"
)

// FormatDeployStageLine formats deploy apply progress text for a stage label.
func FormatDeployStageLine(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" || stage == "<unknown>" {
		return "applying microservice manifest..."
	}
	return fmt.Sprintf("applying microservice manifest... (%s)", stage)
}

// WriteStageLine updates deploy-style stage progress on stderr.
func (u *UI) WriteStageLine(line string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if line == u.lastStageLine {
		return
	}
	if u.quiet {
		return
	}
	if !u.interactive {
		_, _ = fmt.Fprintf(u.ErrOut, "%s\n", line)
		u.lastStageLine = line
		return
	}
	_, _ = io.WriteString(u.ErrOut, clearLine)
	_, _ = io.WriteString(u.ErrOut, line)
	u.lastStageLine = line
}

// WritePercent writes a throttled percent progress bar on stderr.
func (u *UI) WritePercent(label string, percent int, done bool) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.quiet {
		return
	}
	if !u.interactive {
		if done {
			_, _ = fmt.Fprintf(u.ErrOut, "%s: %3d%%\n", label, percent)
		}
		return
	}
	_, _ = io.WriteString(u.ErrOut, clearLine)
	_, _ = fmt.Fprintf(u.ErrOut, "%s: %3d%%", label, percent)
	if done {
		_, _ = io.WriteString(u.ErrOut, "\n")
		u.lastStageLine = ""
	}
}
