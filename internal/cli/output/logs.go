package output

import (
	"io"
	"strings"
)

// FormatLogEntries renders buffered log entries for human output.
func FormatLogEntries(result map[string]any, timestamps bool) string {
	rawEntries, ok := result["entries"].([]any)
	if !ok {
		rawEntries = []any{}
	}
	var b strings.Builder
	for _, raw := range rawEntries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		line := MapValueAsRawString(entry, "line")
		WriteStreamLogLine(&b, MapValueAsString(entry, "ts"), line, timestamps)
	}
	return b.String()
}

// WriteStreamLogLine writes one log line preserving Docker-style spacing.
func WriteStreamLogLine(w io.Writer, ts, line string, timestamps bool) {
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	if timestamps {
		_, _ = io.WriteString(w, ts+" "+line)
		return
	}
	_, _ = io.WriteString(w, line)
}
