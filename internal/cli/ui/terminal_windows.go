//go:build windows

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
