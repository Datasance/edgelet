package output

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"
)

func formatAlignedTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 8, 2, ' ', 0)
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		_, _ = fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	_ = w.Flush()
	return strings.TrimRight(b.String(), "\n")
}

func humanizeCreated(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "<unknown>" {
		return "<unknown>"
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	d := time.Since(ts)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}
