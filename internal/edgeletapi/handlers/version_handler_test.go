package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleVersion_IncludesEmbeddedEngineAndAllowedEngines(t *testing.T) {
	h := &VersionHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/version", nil)
	rec := httptest.NewRecorder()

	h.HandleVersion(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode version response: %v", err)
	}
	requiredKeys := []string{"version", "buildTime", "gitCommit", "embeddedEngine", "allowedContainerEngine"}
	for _, key := range requiredKeys {
		if _, ok := body[key]; !ok {
			t.Fatalf("expected key %q in version response, got %v", key, body)
		}
	}
}
