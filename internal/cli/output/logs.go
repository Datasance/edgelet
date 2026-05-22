package output

import (
	"io"
	"strings"
)

// FormatLogEntries renders buffered log entries for human output.
func FormatLogEntries(result map[string]interface{}, timestamps bool) string {
	rawEntries, _ := result["entries"].([]interface{})
	var b strings.Builder
	for _, raw := range rawEntries {
		entry, ok := raw.(map[string]interface{})
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
