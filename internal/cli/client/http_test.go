package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestCloneRequestWithBody_ReplaysBodyAcrossClones(t *testing.T) {
	payload := []byte(`{"image":"quay.io/skupper/skupper-router:latest"}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://localhost:54321/v3/images:pull", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}

	first, err := cloneRequestWithBody(req)
	if err != nil {
		t.Fatalf("first clone failed: %v", err)
	}
	firstBody, err := io.ReadAll(first.Body)
	if err != nil {
		t.Fatalf("read first clone body failed: %v", err)
	}
	if string(firstBody) != string(payload) {
		t.Fatalf("first clone body mismatch: got %q want %q", string(firstBody), string(payload))
	}

	second, err := cloneRequestWithBody(req)
	if err != nil {
		t.Fatalf("second clone failed: %v", err)
	}
	secondBody, err := io.ReadAll(second.Body)
	if err != nil {
		t.Fatalf("read second clone body failed: %v", err)
	}
	if string(secondBody) != string(payload) {
		t.Fatalf("second clone body mismatch: got %q want %q", string(secondBody), string(payload))
	}
}

func TestCloneRequestWithBody_NoBody(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://localhost:54321/v3/images", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	clone, err := cloneRequestWithBody(req)
	if err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	if clone.Body == nil {
		t.Fatal("expected body to be non-nil")
	}
	body, err := io.ReadAll(clone.Body)
	if err != nil {
		t.Fatalf("failed reading body: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("expected empty body, got %q", string(body))
	}
}
