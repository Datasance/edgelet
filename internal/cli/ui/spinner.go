package ui

import (
	"fmt"
	"io"
	"time"

	"github.com/briandowns/spinner"
)

// Spinner wraps a single stderr spinner tied to this UI instance.
type Spinner struct {
	ui      *UI
	spin    *spinner.Spinner
	message string
	running bool
}

// StartSpinner begins an interactive spinner or prints a plain line in quiet/non-interactive mode.
func (u *UI) StartSpinner(message string) *Spinner {
	u.StopSpinner()
	s := &Spinner{
		ui:      u,
		spin:    spinner.New(spinner.CharSets[14], 100*time.Millisecond),
		message: message,
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.quiet {
		if message != "" {
			_, _ = fmt.Fprintln(u.ErrOut, message)
		}
		return s
	}
	if !u.interactive {
		if message != "" {
			_, _ = fmt.Fprintln(u.ErrOut, message)
		}
		return s
	}
	s.spin.Writer = u.ErrOut
	_ = s.spin.Color("cyan")
	s.spin.Suffix = " " + message
	s.spin.Start()
	s.running = true
	u.activeSpinner = s
	return s
}

// SetSuffix updates spinner suffix text.
func (s *Spinner) SetSuffix(message string) {
	if s == nil || s.ui == nil {
		return
	}
	s.message = message
	s.ui.mu.Lock()
	defer s.ui.mu.Unlock()
	if s.ui.quiet || !s.ui.interactive || !s.running {
		return
	}
	s.spin.Suffix = " " + message
}

// StopSpinner stops the active spinner and clears the progress line.
func (u *UI) StopSpinner() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stopActiveSpinnerLocked()
}

func (u *UI) stopActiveSpinnerLocked() {
	if u.activeSpinner == nil {
		return
	}
	u.activeSpinner.stopLocked()
	u.activeSpinner = nil
}

// Stop stops this spinner instance.
func (s *Spinner) Stop() {
	if s == nil || s.ui == nil {
		return
	}
	s.ui.mu.Lock()
	defer s.ui.mu.Unlock()
	s.stopLocked()
	if s.ui.activeSpinner == s {
		s.ui.activeSpinner = nil
	}
}

func (s *Spinner) stopLocked() {
	if s == nil || s.ui == nil {
		return
	}
	if s.running && s.ui.interactive && !s.ui.quiet {
		s.spin.Stop()
		_, _ = io.WriteString(s.ui.ErrOut, clearLine)
	}
	s.running = false
}

// PauseSpinner stops the spinner temporarily and returns whether it was running.
func (u *UI) PauseSpinner(active *Spinner) bool {
	if active == nil {
		return false
	}
	wasRunning := active.running
	active.Stop()
	return wasRunning
}

// WriteSuccess prints a success marker on stderr and stops any spinner.
func (u *UI) WriteSuccess(message string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stopActiveSpinnerLocked()
	u.writeClearLineLocked()
	if u.colorEnabled() {
		_, _ = fmt.Fprintf(u.ErrOut, "%s✔ %s%s\n", green, message, noFormat)
	} else {
		_, _ = fmt.Fprintf(u.ErrOut, "✔ %s\n", message)
	}
	u.lastStageLine = ""
}

// WriteError prints an error marker on stderr and stops any spinner.
func (u *UI) WriteError(message string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stopActiveSpinnerLocked()
	u.writeClearLineLocked()
	if u.colorEnabled() {
		_, _ = fmt.Fprintf(u.ErrOut, "%s✘ %s%s\n", red, message, noFormat)
	} else {
		_, _ = fmt.Fprintf(u.ErrOut, "✘ %s\n", message)
	}
	u.lastStageLine = ""
}
