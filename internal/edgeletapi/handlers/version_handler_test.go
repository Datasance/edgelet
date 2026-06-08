package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/version"
)

func TestHandleVersion_IncludesEmbeddedEngineAndAllowedEngines(t *testing.T) {
	version.SetBuildInfo("1.2.3-test", "2026-01-01_00:00:00", "abc1234")
	t.Cleanup(func() { version.SetBuildInfo("dev", "unknown", "unknown") })

	h := &VersionHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/version", nil)
	rec := httptest.NewRecorder()

	h.HandleVersion(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode version response: %v", err)
	}
	requiredKeys := []string{"version", "buildTime", "gitCommit", "embeddedEngine", "allowedContainerEngine"}
	for _, key := range requiredKeys {
		if _, ok := body[key]; !ok {
			t.Fatalf("expected key %q in version response, got %v", key, body)
		}
	}
	if body["version"] != "1.2.3-test" {
		t.Fatalf("expected version 1.2.3-test, got %v", body["version"])
	}
	if body["buildTime"] != "2026-01-01_00:00:00" {
		t.Fatalf("expected buildTime 2026-01-01_00:00:00, got %v", body["buildTime"])
	}
	if body["gitCommit"] != "abc1234" {
		t.Fatalf("expected gitCommit abc1234, got %v", body["gitCommit"])
	}
}
