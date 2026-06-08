//go:build !linux || cgo

package cmd

func formatVerboseVersionDetails() string {
	return ""
}
