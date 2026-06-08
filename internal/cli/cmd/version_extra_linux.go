//go:build linux && !cgo

package cmd

import (
	"fmt"
	"strings"

	"github.com/eclipse-iofog/edgelet/pkg/data"
)

func formatVerboseVersionDetails() string {
	var b strings.Builder
	if hash := data.EmbeddedBundleHash(); hash != "" {
		_, _ = fmt.Fprintf(&b, "  embed hash: %s\n", hash)
	}
	if path, err := data.RuntimeBinary(); err == nil {
		_, _ = fmt.Fprintf(&b, "  fat runtime: %s\n", path)
	} else {
		_, _ = b.WriteString("  fat runtime: (not extracted)\n")
	}
	return b.String()
}
