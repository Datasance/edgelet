package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// documentedTopLevelKeys mirrors docs/cli/output-schemas.md golden fixtures.
var documentedTopLevelKeys = map[string][]string{
	"system_status.json": {"connectionToController", "cpuUsage", "edgeletDaemon"},
	"ms_ls.json":         {"items"},
}

var documentedMSItemKeys = []string{
	"uuid", "application", "name", "state", "containerId", "image", "type",
}

func TestGoldenFilesMatchDocumentedSchemas(t *testing.T) {
	for name, keys := range documentedTopLevelKeys {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "golden", name))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("golden is not valid json: %v", err)
			}
			for _, key := range keys {
				if _, ok := decoded[key]; !ok {
					t.Fatalf("documented key %q missing from golden %s", key, name)
				}
			}
			if name == "ms_ls.json" {
				items, ok := decoded["items"].([]any)
				if !ok || len(items) == 0 {
					t.Fatal("expected non-empty items array")
				}
				first, ok := items[0].(map[string]any)
				if !ok {
					t.Fatal("expected object items")
				}
				for _, key := range documentedMSItemKeys {
					if _, ok := first[key]; !ok {
						t.Fatalf("documented item key %q missing from golden %s", key, name)
					}
				}
			}
		})
	}
}
