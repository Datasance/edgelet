package ui

import "fmt"

// FormatRuntimeClassStageLine formats runtimeclass apply progress text.
func FormatRuntimeClassStageLine(stage string) string {
	if stage == "" {
		return "applying runtimeclass manifest..."
	}
	return fmt.Sprintf("applying runtimeclass manifest... (%s)", stage)
}
