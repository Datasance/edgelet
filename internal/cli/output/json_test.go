package output

import (
	"encoding/json"
	"testing"
)

func TestJSONFormatterGoldenRoundTrip(t *testing.T) {
	input := map[string]any{
		"daemon": map[string]any{
			"running": true,
			"pid":     float64(1234),
		},
		"engine": map[string]any{
			"type":    "iofog",
			"healthy": true,
		},
	}

	formatter := JSONFormatter{Indent: "  "}
	raw, err := formatter.Format(input)
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	daemon, ok := decoded["daemon"].(map[string]any)
	if !ok || daemon["running"] != true {
		t.Fatalf("unexpected daemon payload: %#v", decoded["daemon"])
	}
}

func TestParseFormat(t *testing.T) {
	if got, err := ParseFormat("json"); err != nil || got != FormatJSON {
		t.Fatalf("ParseFormat(json) = (%q, %v)", got, err)
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestNewFormatterHumanJSONYAML(t *testing.T) {
	for _, format := range []Format{FormatHuman, FormatJSON, FormatYAML} {
		f, err := NewFormatter(format)
		if err != nil {
			t.Fatalf("NewFormatter(%q): %v", format, err)
		}
		out, err := f.Format(map[string]any{"ok": true})
		if err != nil {
			t.Fatalf("Format(%q): %v", format, err)
		}
		if len(out) == 0 {
			t.Fatalf("expected output for format %q", format)
		}
	}
}

func TestHumanFormatterMapSorted(t *testing.T) {
	out, err := HumanFormatter{}.Format(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if string(out) != "a: 1\nb: 2\n" {
		t.Fatalf("unexpected human output: %q", string(out))
	}
}
