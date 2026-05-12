package handlers

import (
	"encoding/base64"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/auth"
	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/fieldagent"
	"github.com/eclipse-iofog/agent/internal/runtimeapi"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	v3HandlerModuleName = "V3 Api Handler"
)

// V3Handler handles v3 endpoint groups.
type V3Handler struct {
	facade *runtimeapi.Facade
}

// NewV3Handler creates a new v3 handler.
func NewV3Handler() *V3Handler {
	return &V3Handler{facade: runtimeapi.NewFacade()}
}

func (h *V3Handler) HandleSystemProvision(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			ProvisioningKey string `json:"provisioningKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
			return
		}
		if strings.TrimSpace(req.ProvisioningKey) == "" {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "provisioningKey is required", nil)
			return
		}
		if err := h.facade.Provision(req.ProvisioningKey); err != nil {
			writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
			return
		}
		cfg := config.GetInstance()
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"status":    "ok",
			"agentUuid": strings.TrimSpace(cfg.IOFogUUID),
		})
	case http.MethodDelete:
		if err := h.facade.Deprovision(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
	}
}

func (h *V3Handler) HandleSystemReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	if err := config.GetInstance().TriggerReloadCallback(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (h *V3Handler) HandleSystemPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	result := h.facade.Prune()
	writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "ok", "result": result})
}

func (h *V3Handler) HandleSystemGPS(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetInstance()
	switch r.Method {
	case http.MethodGet:
		lat, lon := "0", "0"
		coords := strings.TrimSpace(cfg.GPSCoordinates)
		if coords != "" {
			parts := strings.Split(coords, ",")
			if len(parts) == 2 {
				lat = strings.TrimSpace(parts[0])
				lon = strings.TrimSpace(parts[1])
			}
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"status":    "okay",
			"timestamp": time.Now().UnixMilli(),
			"lat":       lat,
			"lon":       lon,
		})
	case http.MethodPost:
		var req struct {
			Lat interface{} `json:"lat"`
			Lon interface{} `json:"lon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
			return
		}
		lat, err := gpsValueToString(req.Lat)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "lat is required", nil)
			return
		}
		lon, err := gpsValueToString(req.Lon)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "lon is required", nil)
			return
		}
		normalizedCoordinates, err := normalizeGPSLatLon(lat, lon)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		errorsMap := cfg.SetConfig(map[string]interface{}{
			"gps":  "manual",
			"gpsc": normalizedCoordinates,
		})
		if len(errorsMap) > 0 {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid GPS configuration update", map[string]interface{}{"errorMap": errorsMap})
			return
		}
		if err := cfg.TriggerReloadCallback(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
			return
		}
		if err := cfg.TriggerGPSConfigCallback(); err != nil {
			writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "okay"})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
	}
}

func (h *V3Handler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := config.GetInstance()
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"controllerUrl":          cfg.ControllerURL,
			"controllerCert":         cfg.ControllerCert,
			"containerEngine":        cfg.ContainerEngine,
			"dockerUrl":              cfg.DockerURL,
			"networkInterface":       cfg.NetworkInterface,
			"diskLimitGiB":           cfg.DiskLimit,
			"diskDirectory":          cfg.DiskDirectory,
			"memoryLimitMiB":         cfg.MemoryLimit,
			"cpuLimitPercent":        cfg.CPULimit,
			"logDiskLimitGiB":        cfg.LogDiskLimit,
			"logDiskDirectory":       cfg.LogDiskDirectory,
			"logFileCount":           cfg.LogFileCount,
			"logLevel":               cfg.LogLevel,
			"statusFrequencySeconds": cfg.StatusFrequency,
			"changeFrequencySeconds": cfg.ChangeFrequency,
			"postDiagnosticsFreq":    cfg.PostDiagnosticsFreq,
			"deviceScanFrequency":    cfg.DeviceScanFrequency,
			"watchdogEnabled":        cfg.WatchdogEnabled,
			"edgeGuardFrequency":     cfg.EdgeGuardFrequency,
			"gpsMode":                cfg.GPSMode,
			"gpsCoordinates":         cfg.GPSCoordinates,
			"gpsDevice":              cfg.GPSDevice,
			"gpsScanFrequency":       cfg.GPSScanFrequency,
			"arch":                   cfg.Arch,
			"secureMode":             cfg.SecureMode,
			"dockerPruningFrequency": cfg.DockerPruningFrequency,
			"availableDiskThreshold": cfg.AvailableDiskThreshold,
			"upgradeScanFrequency":   cfg.UpgradeScanFrequency,
			"devMode":                cfg.DevMode,
			"timezone":               cfg.TimeZone,
		})
	case http.MethodPatch:
		var req struct {
			Set map[string]interface{} `json:"set"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
			return
		}
		if len(req.Set) == 0 {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing set payload", nil)
			return
		}

		configMap := make(map[string]interface{})
		for key, value := range req.Set {
			if short, ok := configKeyToShortCode(key); ok {
				configMap[short] = fmt.Sprintf("%v", value)
			}
		}
		if len(configMap) == 0 {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "no supported config keys provided", nil)
			return
		}
		errorsMap := config.GetInstance().SetConfig(configMap)
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"status":   "ok",
			"errorMap": errorsMap,
		})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
	}
}

func (h *V3Handler) HandleSystemControllerCert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Certificate string `json:"certificate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	cert := strings.TrimSpace(req.Certificate)
	if cert == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "certificate is required", nil)
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(cert)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "certificate must be a valid base64-encoded PEM certificate", nil)
		return
	}
	if _, err := auth.LoadCertificateFromPEM(decoded); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "decoded certificate must be valid PEM", nil)
		return
	}

	cfg := config.GetInstance()
	certPath := strings.TrimSpace(cfg.ControllerCert)
	if certPath == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "controllerCert path is not configured; use iofog-agent config -ac <path>", nil)
		return
	}
	certDir := filepath.Dir(certPath)
	if certDir != "" && certDir != "." {
		if err := os.MkdirAll(certDir, 0o755); err != nil {
			writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, fmt.Sprintf("failed to prepare certificate directory: %v", err), nil)
			return
		}
	}
	if err := os.WriteFile(certPath, decoded, 0o600); err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, fmt.Sprintf("failed to write certificate file: %v", err), nil)
		return
	}

	errorsMap := cfg.SetConfig(map[string]interface{}{"sec": "on"})
	if len(errorsMap) > 0 {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "failed to enable secure mode", map[string]interface{}{"errorMap": errorsMap})
		return
	}
	if err := cfg.TriggerReloadCallback(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"status":               "ok",
		"updatedKey":           "controllerCert",
		"certificatePath":      certPath,
		"secureMode":           "on",
		"certificateInstalled": true,
	})
}

func (h *V3Handler) HandleSystemConfigSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	profileInput := strings.TrimSpace(req.Profile)
	if profileInput == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "profile is required", nil)
		return
	}
	profile, err := utils.ParseConfigSwitcherState(strings.ToLower(profileInput))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "profile must be one of dev|prod|def", nil)
		return
	}
	cfg := config.GetInstance()
	oldProfile := cfg.GetCurrentProfile().FullValue()
	if err := cfg.SwitchProfile(profile); err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	if err := cfg.TriggerReloadCallback(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"status":     "ok",
		"oldProfile": oldProfile,
		"profile":    profile.FullValue(),
	})
}

func (h *V3Handler) HandleMicroserviceConfigSelf(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeAPIError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "missing bearer token", nil)
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	result, err := auth.ValidateLocalJWT(token)
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "invalid JWT token", nil)
		return
	}
	microserviceUUID, ok := claimMicroserviceUUID(result.Claims)
	if !ok {
		writeAPIError(w, http.StatusForbidden, ErrCodeForbidden, "microservice identity claim is required", nil)
		return
	}

	configString, exists := fieldagent.GetInstance().GetContainerConfig(microserviceUUID)
	if !exists || strings.TrimSpace(configString) == "" {
		writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "microservice config not found", nil)
		return
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(configString), &payload); err != nil {
		// Keep v2-aligned arbitrary payload behavior even when payload is not strict JSON.
		payload = configString
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"microserviceUuid": microserviceUUID,
		"config":           payload,
	})
}

func (h *V3Handler) HandleMicroservices(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v3/ms" {
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"items": h.facade.ListRuntimeMicroservices(),
		})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v3/ms/")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing microservice id", nil)
		return
	}

	// Keep write operations explicitly unimplemented until runtime action mapping is finalized.
	if strings.Contains(id, "/") {
		writeAPIError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "not implemented", nil)
		return
	}
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "not implemented", nil)
		return
	}

	item, err := h.facade.GetRuntimeMicroservice(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "microservice not found", nil)
		return
	}
	writeSuccess(w, http.StatusOK, item)
}

func (h *V3Handler) HandleDeployMicroservicesApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Manifest   string `json:"manifest"`
		SourceName string `json:"sourceName"`
		DryRun     bool   `json:"dryRun"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Manifest) == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "manifest is required", nil)
		return
	}
	id, manifestDoc, err := h.facade.ApplyLocalManifest(req.Manifest, req.SourceName, req.DryRun)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"accepted":     true,
		"deploymentId": id,
		"dryRun":       req.DryRun,
		"kind":         manifestDoc.Kind,
		"name":         manifestDoc.Metadata.Name,
		"image":        manifestDoc.Spec.Image,
	})
}

func (h *V3Handler) HandleDeployMicroservicesValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Manifest string `json:"manifest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Manifest) == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "manifest is required", nil)
		return
	}
	doc, err := h.facade.ParseAndValidateLocalManifest(req.Manifest)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"valid":      true,
		"kind":       doc.Kind,
		"name":       doc.Metadata.Name,
		"apiVersion": doc.APIVersion,
		"image":      doc.Spec.Image,
	})
}

func (h *V3Handler) HandleDeployMicroservices(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v3/deploy/microservices" {
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		list, err := h.facade.ListLocalDeployments()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{"items": list})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v3/deploy/microservices/")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing deployment id", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := h.facade.GetLocalDeployment(id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "not found", nil)
				return
			}
			writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := h.facade.DeleteLocalDeployment(id); err != nil {
			writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
	}
}

func (h *V3Handler) HandleDeployRegistriesApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Manifest string `json:"manifest"`
		DryRun   bool   `json:"dryRun"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Manifest) == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "manifest is required", nil)
		return
	}
	reg, err := h.facade.ApplyLocalRegistryManifest(req.Manifest, req.DryRun)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"accepted": true,
		"dryRun":   req.DryRun,
		"registry": reg,
	})
}

func (h *V3Handler) HandleDeployRegistriesValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Manifest string `json:"manifest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Manifest) == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "manifest is required", nil)
		return
	}
	doc, err := h.facade.ParseAndValidateLocalRegistryManifest(req.Manifest)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"valid":      true,
		"apiVersion": doc.APIVersion,
		"kind":       doc.Kind,
		"id":         doc.Spec.ID,
		"url":        doc.Spec.URL,
	})
}

func (h *V3Handler) HandleDeployRegistries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	items, err := h.facade.ListRegistries()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (h *V3Handler) HandleAuthTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	items, err := store.GetInstance().ListServiceAccountTokens()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (h *V3Handler) HandleAuthTokensRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		JTI string `json:"jti"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.JTI) == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "jti is required", nil)
		return
	}
	if err := store.GetInstance().RevokeServiceAccountToken(req.JTI, time.Now().Unix()); err != nil {
		logging.LogError(v3HandlerModuleName, "Failed to revoke token", err)
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func configKeyToShortCode(key string) (string, bool) {
	normalized := strings.TrimSpace(key)
	normalized = strings.TrimPrefix(normalized, "--")
	normalized = strings.TrimPrefix(normalized, "-")

	switch normalized {
	case "a", "controllerUrl":
		return "a", true
	case "ac", "controllerCert":
		return "ac", true
	case "ce", "containerEngine":
		return "ce", true
	case "c", "dockerUrl":
		return "c", true
	case "n", "networkInterface":
		return "n", true
	case "d", "diskLimitGiB":
		return "d", true
	case "dl", "diskDirectory":
		return "dl", true
	case "m", "memoryLimitMiB":
		return "m", true
	case "p", "cpuLimitPercent":
		return "p", true
	case "l", "logDiskLimitGiB":
		return "l", true
	case "ld", "logDiskDirectory":
		return "ld", true
	case "lc", "logFileCount":
		return "lc", true
	case "ll", "logLevel":
		return "ll", true
	case "sf", "statusFrequencySeconds":
		return "sf", true
	case "cf", "changeFrequencySeconds":
		return "cf", true
	case "df", "postDiagnosticsFreq":
		return "df", true
	case "sd", "deviceScanFrequency":
		return "sd", true
	case "idc", "watchdogEnabled":
		return "idc", true
	case "egf", "edgeGuardFrequency":
		return "egf", true
	case "gps", "gpsMode":
		return "gps", true
	case "gpsc", "gpsCoordinates":
		return "gpsc", true
	case "gpsd", "gpsDevice":
		return "gpsd", true
	case "gpsf", "gpsScanFrequency":
		return "gpsf", true
	case "ft", "arch":
		return "ft", true
	case "sec", "secureMode":
		return "sec", true
	case "pf", "dockerPruningFrequency":
		return "pf", true
	case "dt", "availableDiskThreshold":
		return "dt", true
	case "uf", "upgradeScanFrequency":
		return "uf", true
	case "dev", "devMode":
		return "dev", true
	case "tz", "timezone":
		return "tz", true
	default:
		return "", false
	}
}

func gpsValueToString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return "", fmt.Errorf("empty")
		}
		return s, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	default:
		return "", fmt.Errorf("unsupported")
	}
}

func normalizeGPSLatLon(lat, lon string) (string, error) {
	latValue, err := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	if err != nil {
		return "", fmt.Errorf("lat must be a valid number")
	}
	lonValue, err := strconv.ParseFloat(strings.TrimSpace(lon), 64)
	if err != nil {
		return "", fmt.Errorf("lon must be a valid number")
	}
	if latValue < -90 || latValue > 90 {
		return "", fmt.Errorf("lat must be between -90 and 90")
	}
	if lonValue < -180 || lonValue > 180 {
		return "", fmt.Errorf("lon must be between -180 and 180")
	}
	return fmt.Sprintf("%.5f,%.5f", latValue, lonValue), nil
}

func claimMicroserviceUUID(claims map[string]interface{}) (string, bool) {
	iofog, ok := claims["iofog.org"].(map[string]interface{})
	if !ok {
		return "", false
	}
	ms, ok := iofog["microservice"].(map[string]interface{})
	if !ok {
		return "", false
	}
	uuid, _ := ms["uuid"].(string)
	uuid = strings.TrimSpace(uuid)
	return uuid, uuid != ""
}
