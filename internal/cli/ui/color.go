package ui

import "os"

const (
	noFormat = "\033[0m"
	green    = "\033[38;5;28m"
	red      = "\033[38;5;1m"
)

func (u *UI) colorEnabled() bool {
	if u == nil || u.quiet || u.noColor {
		return false
	}
	if isTruthyEnv(os.Getenv(envNoColor)) {
		return false
	}
	return true
}
