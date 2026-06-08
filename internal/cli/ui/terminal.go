package ui

import "os"

var isTerminalFile = func(f *os.File) bool {
	return false
}
