//go:build !linux || !full || cgo

package cmd

func formatVerboseVersionDetails() string {
	return ""
}
