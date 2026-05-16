package gps

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-iofog/agent/internal/config"
)

func TestWebHandlerUpdateCoordinates_ParsesLatLon(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","lat":41.0151,"lon":28.9795}`))
	}))
	defer server.Close()

	prevURL := ipAPIURL
	ipAPIURL = server.URL
	t.Cleanup(func() { ipAPIURL = prevURL })

	cfg := config.GetInstance()
	cfg.GPSCoordinates = ""

	handler := NewWebHandler(nil)
	if err := handler.UpdateCoordinates(); err != nil {
		t.Fatalf("UpdateCoordinates returned error: %v", err)
	}
	if cfg.GPSCoordinates != "41.01510,28.97950" {
		t.Fatalf("unexpected coordinates: %s", cfg.GPSCoordinates)
	}
}

func TestWebHandlerUpdateCoordinates_MissingLatLonFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	prevURL := ipAPIURL
	ipAPIURL = server.URL
	t.Cleanup(func() { ipAPIURL = prevURL })

	cfg := config.GetInstance()
	cfg.GPSCoordinates = "10.00000,20.00000"

	handler := NewWebHandler(nil)
	if err := handler.UpdateCoordinates(); err == nil {
		t.Fatal("expected UpdateCoordinates to fail when lat/lon are missing")
	}
	if cfg.GPSCoordinates != "10.00000,20.00000" {
		t.Fatalf("coordinates should remain unchanged on failure, got %s", cfg.GPSCoordinates)
	}
}
