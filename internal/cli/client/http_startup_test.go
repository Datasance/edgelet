package client

import (
	"net/http"
	"testing"
)

func TestNormalizeAPIErrorLocalAPIStarting(t *testing.T) {
	code, message := normalizeAPIError(http.StatusServiceUnavailable, `{"reason":"local_api_listener_not_ready"}`, "")
	if code != ErrCodeLocalAPIStarting {
		t.Fatalf("expected %s, got %s", ErrCodeLocalAPIStarting, code)
	}
	if message == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestNormalizeAPIErrorControllerOffline(t *testing.T) {
	code, _ := normalizeAPIError(http.StatusServiceUnavailable, `{"reason":"controller_unreachable"}`, "")
	if code != ErrCodeControllerOffline {
		t.Fatalf("expected %s, got %s", ErrCodeControllerOffline, code)
	}
}
