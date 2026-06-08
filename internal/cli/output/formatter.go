package output

import "fmt"

// Format selects CLI data encoding.
type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// ParseFormat normalizes a format flag value.
func ParseFormat(raw string) (Format, error) {
	switch Format(raw) {
	case "", FormatHuman:
		return FormatHuman, nil
	case FormatJSON:
		return FormatJSON, nil
	case FormatYAML:
		return FormatYAML, nil
	default:
		return "", fmt.Errorf("unsupported output format %q", raw)
	}
}

// Formatter encodes command data for stdout.
type Formatter interface {
	Format(v any) ([]byte, error)
}

// NewFormatter returns a formatter for the requested output mode.
func NewFormatter(format Format) (Formatter, error) {
	switch format {
	case FormatHuman:
		return HumanFormatter{}, nil
	case FormatJSON:
		return JSONFormatter{Indent: "  "}, nil
	case FormatYAML:
		return YAMLFormatter{}, nil
	default:
		return nil, fmt.Errorf("unsupported output format %q", format)
	}
}

// IsStructured reports whether output goes to stdout as machine-readable data only.
func (f Format) IsStructured() bool {
	return f == FormatJSON || f == FormatYAML
}
