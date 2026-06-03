package ui

import (
	"fmt"
	"strings"
)

// FormatControlPlaneStageLine formats control plane deploy apply progress text.
func FormatControlPlaneStageLine(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" || stage == "<unknown>" {
		return "applying control plane manifest..."
	}
	return fmt.Sprintf("applying control plane manifest... (%s)", stage)
}
