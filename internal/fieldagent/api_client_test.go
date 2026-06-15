package fieldagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIClient_BadRequestIncludesResponseBody(t *testing.T) {
	const wantDetail = "Required field 'internal' is missing"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"` + wantDetail + `"}`))
	}))
	t.Cleanup(srv.Close)

	client := &APIClient{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
	}

	_, err := client.Request(context.Background(), "provision", POST, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bad request: status 400") {
		t.Fatalf("unexpected error prefix: %v", err)
	}
	if !strings.Contains(err.Error(), wantDetail) {
		t.Fatalf("expected controller body in error, got: %v", err)
	}
}

func TestControllerHTTPError(t *testing.T) {
	err := controllerHTTPError(http.StatusBadRequest, "bad request", `{"message":"invalid"}`)
	if err == nil || !strings.Contains(err.Error(), "bad request: status 400: {\"message\":\"invalid\"}") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = controllerHTTPError(http.StatusForbidden, "forbidden", "")
	if err == nil || !strings.Contains(err.Error(), "forbidden: status 403") {
		t.Fatalf("unexpected error: %v", err)
	}
}
