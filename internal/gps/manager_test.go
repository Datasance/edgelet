package gps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/eclipse-iofog/agent/internal/config"
)

func TestGetGPSMode_CaseInsensitive(t *testing.T) {
	cfg := config.GetInstance()
	m := &Manager{config: cfg}

	cfg.GPSMode = "manual"
	if got := m.getGpsMode(); got != ModeManual {
		t.Fatalf("expected MANUAL, got %s", got)
	}

	cfg.GPSMode = "DyNaMiC"
	if got := m.getGpsMode(); got != ModeDynamic {
		t.Fatalf("expected DYNAMIC, got %s", got)
	}

	cfg.GPSMode = "off"
	if got := m.getGpsMode(); got != ModeOff {
		t.Fatalf("expected OFF, got %s", got)
	}
}

func TestStartCoordinateUpdateScheduler_DisabledWhenFrequencyZero(t *testing.T) {
	cfg := config.GetInstance()
	cfg.GPSScanFrequency = 0

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	m := &Manager{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
	}
	m.startCoordinateUpdateScheduler()
	if m.updateTicker != nil {
		t.Fatal("expected update ticker to stay nil when GPSScanFrequency <= 0")
	}
}

func TestInitializeGPS_DynamicWithoutDeviceFallsBackToManual(t *testing.T) {
	cfg := config.GetInstance()
	cfg.GPSMode = "dynamic"
	cfg.GPSDevice = ""

	m := &Manager{
		status: NewStatus(),
		config: cfg,
	}
	if err := m.initializeGps(); err != nil {
		t.Fatalf("initializeGps returned error: %v", err)
	}
	if m.status.GetHealthStatus() != HealthStatusHealthy {
		t.Fatalf("expected HEALTHY status, got %s", m.status.GetHealthStatus())
	}
}

func TestUpdateCoordinates_AutoModeTriggersGPSCallbackOnSuccess(t *testing.T) {
	cfg := config.GetInstance()
	cfg.GPSMode = "auto"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","lat":41.0151,"lon":28.9795}`))
	}))
	defer server.Close()

	originalURL := ipAPIURL
	ipAPIURL = server.URL
	t.Cleanup(func() { ipAPIURL = originalURL })

	var callbackCount int32
	cfg.SetGPSConfigCallback(func() error {
		atomic.AddInt32(&callbackCount, 1)
		return nil
	})

	m := &Manager{
		status: NewStatus(),
		config: cfg,
	}
	m.webHandler = NewWebHandler(m)

	m.updateCoordinates()

	if got := atomic.LoadInt32(&callbackCount); got != 1 {
		t.Fatalf("expected GPS callback to be triggered once, got %d", got)
	}
}

func TestUpdateCoordinates_AutoModeDoesNotTriggerGPSCallbackOnFailure(t *testing.T) {
	cfg := config.GetInstance()
	cfg.GPSMode = "auto"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	originalURL := ipAPIURL
	ipAPIURL = server.URL
	t.Cleanup(func() { ipAPIURL = originalURL })

	var callbackCount int32
	cfg.SetGPSConfigCallback(func() error {
		atomic.AddInt32(&callbackCount, 1)
		return nil
	})

	m := &Manager{
		status: NewStatus(),
		config: cfg,
	}
	m.webHandler = NewWebHandler(m)

	m.updateCoordinates()

	if got := atomic.LoadInt32(&callbackCount); got != 0 {
		t.Fatalf("expected GPS callback not to be triggered, got %d", got)
	}
}
