//go:build unix

package ui

import (
	"os"

	"github.com/mattn/go-isatty"
)

func init() {
	isTerminalFile = func(f *os.File) bool {
		return isatty.IsTerminal(f.Fd())
	}
}
