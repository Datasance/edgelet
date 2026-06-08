package ui

import (
	"io"
	"os"
	"strings"
)

const (
	envNoColor = "NO_COLOR"
	envCI      = "CI"
	envTerm    = "TERM"
)

// Options configures interactive CLI output behavior.
type Options struct {
	Quiet    bool
	NoColor  bool
	ForceTTY bool
}

// IsInteractive reports whether progress/spinner UX should be used for w.
func IsInteractive(w io.Writer, opts Options) bool {
	if opts.Quiet {
		return false
	}
	if opts.NoColor || isTruthyEnv(os.Getenv(envNoColor)) {
		return false
	}
	if isTruthyEnv(os.Getenv(envCI)) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(envTerm)), "dumb") {
		return false
	}
	if opts.ForceTTY {
		return true
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isTerminalFile(f)
}

func isTruthyEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
