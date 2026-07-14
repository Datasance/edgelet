package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/runtimestate"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

type runtimeDrainStubEngine struct {
	engine.ContainerEngine
}

func (runtimeDrainStubEngine) GetRunningContainers() ([]engine.Container, error) {
	return nil, nil
}

func TestHandleRuntimeDrain_EngineNotReady(t *testing.T) {
	setupConfigForGPSTests(t)
	runtimestate.ResetForTests()
	processmanager.ResetProcessManagerEngineForTest()
	t.Cleanup(func() {
		runtimestate.ResetForTests()
		processmanager.ResetProcessManagerEngineForTest()
	})

	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/runtime/drain", nil)
	rec := httptest.NewRecorder()
	handler.HandleRuntimeDrain(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleRuntimeDrain_AcceptsWiredEngineWhenNotReady(t *testing.T) {
	setupConfigForGPSTests(t)
	runtimestate.ResetForTests()
	runtimestate.GetState().SetEngineReady(false)
	t.Cleanup(func() {
		runtimestate.ResetForTests()
		processmanager.SetQuiesced(false)
	})

	pm := processmanager.NewProcessManagerForHandlerTest(runtimeDrainStubEngine{})
	restore := processmanager.SetInstanceForTest(pm)
	t.Cleanup(restore)

	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/runtime/drain", nil)
	rec := httptest.NewRecorder()
	handler.HandleRuntimeDrain(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when engine wired but engineReady=false, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleRuntimeDrain_MethodNotAllowed(t *testing.T) {
	setupConfigForGPSTests(t)
	handler := NewEdgeletAPIHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/runtime/drain", nil)
	rec := httptest.NewRecorder()
	handler.HandleRuntimeDrain(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", rec.Code, rec.Body.String())
	}
}
