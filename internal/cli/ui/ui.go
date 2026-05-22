package ui

import (
	"io"
	"os"
	"sync"
)

const clearLine = "\r\x1b[K"

// UI centralizes CLI user-facing output on stderr and data on stdout.
type UI struct {
	Out    io.Writer
	ErrOut io.Writer

	interactive bool
	quiet       bool
	noColor     bool

	mu sync.Mutex

	activeSpinner *Spinner
	lastStageLine string
}

// New creates a UI with TTY detection on stderr.
func New(opts Options) *UI {
	errOut := os.Stderr
	return &UI{
		Out:         os.Stdout,
		ErrOut:      errOut,
		interactive: IsInteractive(errOut, opts),
		quiet:       opts.Quiet,
		noColor:     opts.NoColor,
	}
}

// NewWithWriters creates a UI for tests with explicit writers.
func NewWithWriters(out, errOut io.Writer, opts Options) *UI {
	return &UI{
		Out:         out,
		ErrOut:      errOut,
		interactive: IsInteractive(errOut, opts),
		quiet:       opts.Quiet,
		noColor:     opts.NoColor,
	}
}

// Interactive reports whether spinner/progress overwrite mode is active.
func (u *UI) Interactive() bool {
	return u.interactive
}

// ClearProgressLine clears the current stderr progress line.
func (u *UI) ClearProgressLine() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.writeClearLineLocked()
	u.lastStageLine = ""
}

func (u *UI) writeClearLineLocked() {
	if !u.interactive || u.quiet {
		return
	}
	_, _ = io.WriteString(u.ErrOut, clearLine)
}
