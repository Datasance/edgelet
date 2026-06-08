package output

import (
	"fmt"
	"slices"
	"strings"
)

// HumanFormatter renders simple human-readable key-value or scalar output.
type HumanFormatter struct{}

func (HumanFormatter) Format(v any) ([]byte, error) {
	switch typed := v.(type) {
	case nil:
		return nil, nil
	case string:
		if !strings.HasSuffix(typed, "\n") {
			typed += "\n"
		}
		return []byte(typed), nil
	case fmt.Stringer:
		return []byte(typed.String() + "\n"), nil
	case map[string]any:
		return []byte(formatMap(typed)), nil
	default:
		return []byte(fmt.Sprintf("%v\n", typed)), nil
	}
}

func formatMap(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		_, _ = fmt.Fprintf(&b, "%s: %v\n", k, m[k])
	}
	return b.String()
}
