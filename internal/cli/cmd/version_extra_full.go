//go:build linux && full && !cgo

package cmd

import (
	"fmt"
	"strings"

	"github.com/datasance/edgelet/pkg/data"
)

func formatVerboseVersionDetails() string {
	var b strings.Builder
	if hash := data.EmbeddedBundleHash(); hash != "" {
		fmt.Fprintf(&b, "  embed hash: %s\n", hash)
	}
	if path, err := data.RuntimeBinary(); err == nil {
		fmt.Fprintf(&b, "  fat runtime: %s\n", path)
	} else {
		b.WriteString("  fat runtime: (not extracted)\n")
	}
	return b.String()
}
