//revive:disable:nested-structs
package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleImages_RejectsRequestBody(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/images", bytes.NewBufferString(`{"unexpected":true}`))
	rec := httptest.NewRecorder()

	handler.HandleImages(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "request body is not allowed") {
		t.Fatalf("expected body validation message, got: %s", rec.Body.String())
	}
}

func TestHandleImagePull_RejectsUnknownFields(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/images:pull", bytes.NewBufferString(`{"image":"nginx:latest","extra":"x"}`))
	rec := httptest.NewRecorder()

	handler.HandleImagePull(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleImagePull_RequiresImage(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/images:pull", bytes.NewBufferString(`{"platform":"linux/arm64"}`))
	rec := httptest.NewRecorder()

	handler.HandleImagePull(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "image is required") {
		t.Fatalf("expected missing image message, got: %s", rec.Body.String())
	}
}

func TestHandleImagePull_AsyncReturnsOperationID(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/images:pull", bytes.NewBufferString(`{"image":"nginx:latest","async":true}`))
	rec := httptest.NewRecorder()

	handler.HandleImagePull(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success envelope, got: %s", rec.Body.String())
	}
	if _, ok := envelope.Data["operationId"]; !ok {
		t.Fatalf("expected operationId in response, got: %v", envelope.Data)
	}
}

func TestHandleImagePullStatus_MissingOperationID(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/images:pull/", nil)
	rec := httptest.NewRecorder()

	handler.HandleImagePullStatus(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleImageLoad_AsyncAccepted(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/images:load", bytes.NewBufferString(`{"path":"/tmp/test.tar"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleImageLoad(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	operationID, ok := envelope.Data["operationId"].(string)
	if !ok {
		t.Fatal("type assertion failed for operationID")
	}
	if strings.TrimSpace(operationID) == "" {
		t.Fatalf("expected operationId, got %#v", envelope.Data)
	}
}

func TestHandleImageLoad_RequiresPath(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/images:load", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()

	handler.HandleImageLoad(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "path is required") {
		t.Fatalf("expected missing path message, got: %s", rec.Body.String())
	}
}

func TestHandleImagePrune_MethodNotAllowed(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/images:prune", nil)
	rec := httptest.NewRecorder()

	handler.HandleImagePrune(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleImagePrune_InvalidMode(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/images:prune?mode=bad", nil)
	rec := httptest.NewRecorder()

	handler.HandleImagePrune(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "only mode=dangling") {
		t.Fatalf("expected dangling-only validation message, got: %s", rec.Body.String())
	}
}

func TestHandleImagePullStatus_NotFound(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/images:pull/non-existent", nil)
	rec := httptest.NewRecorder()

	handler.HandleImagePullStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleImageRemove_RequiresSelector(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/images:remove", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()

	handler.HandleImageRemove(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "selector is required") {
		t.Fatalf("expected selector validation message, got: %s", rec.Body.String())
	}
}

func TestHandleImagePullAsyncThenStatusEventuallyTerminal(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	startReq := httptest.NewRequest(http.MethodPost, "/v1/images:pull", bytes.NewBufferString(`{"image":"nginx:latest","async":true}`))
	startRec := httptest.NewRecorder()
	handler.HandleImagePull(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d body=%s", startRec.Code, startRec.Body.String())
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			OperationID string `json:"operationId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(startRec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode start response: %v", err)
	}
	if strings.TrimSpace(envelope.Data.OperationID) == "" {
		t.Fatalf("missing operation id in start response: %s", startRec.Body.String())
	}

	// Pull may fail quickly in tests due missing process manager engine; ensure status endpoint becomes terminal.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		statusReq := httptest.NewRequest(http.MethodGet, "/v1/images:pull/"+envelope.Data.OperationID, nil)
		statusRec := httptest.NewRecorder()
		handler.HandleImagePullStatus(statusRec, statusReq)
		if statusRec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
		}
		if strings.Contains(statusRec.Body.String(), "\"status\":\"succeeded\"") || strings.Contains(statusRec.Body.String(), "\"status\":\"failed\"") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("operation did not reach terminal state within timeout")
}

func TestHandleDeployMicroservicesApply_AsyncAccepted(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := newDeployApplyMultipartRequest(t, map[string]string{
		"async":  "false",
		"dryRun": "true",
	})
	rec := httptest.NewRecorder()

	handler.HandleDeployMicroservicesApply(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success response, got %s", rec.Body.String())
	}
	operationID, ok := envelope.Data["operationId"].(string)
	if !ok {
		t.Fatal("type assertion failed for operationID")
	}
	if strings.TrimSpace(operationID) == "" {
		t.Fatalf("expected operationId in response, got: %v", envelope.Data)
	}
	status, ok := envelope.Data["status"].(string)
	if !ok {
		t.Fatal("type assertion failed for status")
	}
	if !strings.EqualFold(status, "running") {
		t.Fatalf("expected running status, got: %v", envelope.Data)
	}
}

func TestHandleDeployMicroservicesApplyStatus_NotFound(t *testing.T) {
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/deploy/microservices:apply/does-not-exist", nil)
	rec := httptest.NewRecorder()

	handler.HandleDeployMicroservicesApplyStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeployMicroservicesApplyStatus_EventuallySucceeded(t *testing.T) {
	ensureStoreDBOpen(t)
	handler := NewEdgeletAPIHandler()
	startReq := newDeployApplyMultipartRequest(t, map[string]string{
		"async":  "true",
		"dryRun": "true",
	})
	startRec := httptest.NewRecorder()
	handler.HandleDeployMicroservicesApply(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d body=%s", startRec.Code, startRec.Body.String())
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			OperationID string `json:"operationId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(startRec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode start response: %v", err)
	}
	if strings.TrimSpace(envelope.Data.OperationID) == "" {
		t.Fatalf("missing operation id in start response: %s", startRec.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		statusReq := httptest.NewRequest(http.MethodGet, "/v1/deploy/microservices:apply/"+envelope.Data.OperationID, nil)
		statusRec := httptest.NewRecorder()
		handler.HandleDeployMicroservicesApplyStatus(statusRec, statusReq)
		if statusRec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
		}
		if strings.Contains(statusRec.Body.String(), "\"status\":\"succeeded\"") {
			if !strings.Contains(statusRec.Body.String(), "\"stage\":\"done\"") {
				t.Fatalf("expected done stage on success, got %s", statusRec.Body.String())
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("deploy operation did not reach succeeded state within timeout")
}

func newDeployApplyMultipartRequest(t *testing.T, fields map[string]string) *http.Request {
	t.Helper()
	manifest := strings.TrimSpace(`
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: router
spec:
  image: quay.io/skupper/skupper-router:latest
`) + "\n"
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("manifest", "manifest.yaml")
	if err != nil {
		t.Fatalf("failed to create manifest part: %v", err)
	}
	if _, err := fileWriter.Write([]byte(manifest)); err != nil {
		t.Fatalf("failed to write manifest payload: %v", err)
	}
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatalf("failed to write form field %q: %v", k, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/deploy/microservices:apply", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
