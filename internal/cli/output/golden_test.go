package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGoldenSystemStatusJSON(t *testing.T) {
	input := map[string]any{
		"edgeletDaemon":          "running",
		"connectionToController": "ok",
		"cpuUsage":               float64(12),
	}
	assertGoldenJSON(t, "system_status.json", input)
}

func TestGoldenMSListJSON(t *testing.T) {
	input := map[string]any{
		"items": []any{
			map[string]any{
				"uuid":        "abc-123",
				"application": "local",
				"name":        "demo",
				"state":       "running",
				"containerId": "c1",
				"image":       "demo:latest",
				"type":        "standard",
			},
		},
	}
	assertGoldenJSON(t, "ms_ls.json", input)
}

func assertGoldenJSON(t *testing.T, name string, input map[string]any) {
	t.Helper()
	formatter := JSONFormatter{Indent: "  "}
	raw, err := formatter.Format(input)
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	goldenPath := filepath.Join("testdata", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run UPDATE_GOLDEN=1 go test)", goldenPath, err)
	}
	if string(raw) != string(expected) {
		t.Fatalf("golden mismatch for %s\n got: %s\nwant: %s", name, string(raw), string(expected))
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("golden is not valid json: %v", err)
	}
}
