package fieldagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/eclipse-iofog/agent/internal/auth"
	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
)

func TestPostGPSConfig_SendsConfigGPSPayload(t *testing.T) {
	cfg := config.GetInstance()
	cfg.IOFogUUID = "agent-1"
	cfg.GPSCoordinates = "41.01510,28.97950"

	var requestCount int32
	var latitude float64
	var longitude float64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH method, got %s", r.Method)
		}
		if r.URL.Path != "/agent/config/gps" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		atomic.AddInt32(&requestCount, 1)

		var body map[string]float64
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		latitude = body["latitude"]
		longitude = body["longitude"]

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    context.Background(),
		apiClient: &APIClient{
			baseURL:    server.URL,
			httpClient: server.Client(),
			jwtManager: auth.GetJWTManager(),
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)

	if err := fa.postGPSConfig(); err != nil {
		t.Fatalf("postGPSConfig returned error: %v", err)
	}

	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("expected exactly one request, got %d", got)
	}
	if latitude != 41.01510 || longitude != 28.97950 {
		t.Fatalf("unexpected payload latitude/longitude: %f,%f", latitude, longitude)
	}
}

func TestPostGPSConfig_SkipsInvalidCoordinates(t *testing.T) {
	cfg := config.GetInstance()
	cfg.IOFogUUID = "agent-1"
	cfg.GPSCoordinates = "invalid"

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    context.Background(),
		apiClient: &APIClient{
			baseURL:    server.URL,
			httpClient: server.Client(),
			jwtManager: auth.GetJWTManager(),
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)

	if err := fa.postGPSConfig(); err != nil {
		t.Fatalf("expected nil error for invalid coordinates skip, got %v", err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 0 {
		t.Fatalf("expected no outbound request for invalid coordinates, got %d", got)
	}
}

func TestInstanceGPSConfigUpdated_InvokesPostGPSConfig(t *testing.T) {
	cfg := config.GetInstance()
	cfg.IOFogUUID = "agent-1"
	cfg.GPSCoordinates = "10.00000,20.00000"

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agent/config/gps" {
			atomic.AddInt32(&requestCount, 1)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    context.Background(),
		apiClient: &APIClient{
			baseURL:    server.URL,
			httpClient: server.Client(),
			jwtManager: auth.GetJWTManager(),
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)

	if err := fa.InstanceGPSConfigUpdated(); err != nil {
		t.Fatalf("InstanceGPSConfigUpdated returned error: %v", err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("expected one config/gps request, got %d", got)
	}
}
