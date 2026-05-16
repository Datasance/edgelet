package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/utils"
)

func TestHandleSystemGPSPost_SetsManualModeAndPersistsCoordinates(t *testing.T) {
	cfg := setupConfigForGPSTests(t)
	reloadCallbackCalled := false
	gpsCallbackCalled := false
	cfg.SetReloadCallback(func() error {
		reloadCallbackCalled = true
		return nil
	})
	cfg.SetGPSConfigCallback(func() error {
		gpsCallbackCalled = true
		return nil
	})

	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodPost, "/v3/system/gps", bytes.NewBufferString(`{"lat":"41.0151","lon":"28.9795"}`))
	rec := httptest.NewRecorder()
	handler.HandleSystemGPS(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if cfg.GPSMode != "manual" {
		t.Fatalf("expected gps mode manual, got %q", cfg.GPSMode)
	}
	if cfg.GPSCoordinates != "41.01510,28.97950" {
		t.Fatalf("expected normalized coordinates, got %q", cfg.GPSCoordinates)
	}
	if !reloadCallbackCalled {
		t.Fatal("expected reload callback to be called")
	}
	if !gpsCallbackCalled {
		t.Fatal("expected GPS callback to be called")
	}
}

func TestHandleSystemGPSPost_RejectsOutOfRangeLatitude(t *testing.T) {
	_ = setupConfigForGPSTests(t)
	handler := NewV3Handler()
	req := httptest.NewRequest(http.MethodPost, "/v3/system/gps", bytes.NewBufferString(`{"lat":"95","lon":"28.9"}`))
	rec := httptest.NewRecorder()
	handler.HandleSystemGPS(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}
	if success, _ := envelope["success"].(bool); success {
		t.Fatal("expected success=false in error envelope")
	}
}

func setupConfigForGPSTests(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.GetInstance()

	tmpDir := t.TempDir()
	cfg.SetConfigPath(filepath.Join(tmpDir, "config.yaml"))
	cfg.SetCurrentProfile(utils.ConfigSwitcherStateDefault)

	yamlCfg := models.NewYamlConfig()
	yamlCfg.CurrentProfile = utils.ConfigSwitcherStateDefault.FullValue()
	profile := models.NewProfileConfig()
	yamlCfg.Profiles[utils.ConfigSwitcherStateDefault.FullValue()] = profile
	cfg.SetYamlConfig(yamlCfg)

	cfg.GPSMode = "auto"
	cfg.GPSCoordinates = "0.00000,0.00000"
	return cfg
}
