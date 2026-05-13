package handlers

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/auth"
	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/fieldagent"
	"github.com/eclipse-iofog/agent/internal/processmanager"
	"github.com/eclipse-iofog/agent/internal/runtimeapi"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	v3HandlerModuleName = "V3 Api Handler"
)

// V3Handler handles v3 endpoint groups.
type V3Handler struct {
	facade       *runtimeapi.Facade
	execSessions map[string]*localExecSession
	execMu       sync.RWMutex
	pullOps      map[string]*imagePullOperation
	pullMu       sync.RWMutex
	deployOps    map[string]*deployApplyOperation
	deployMu     sync.RWMutex
}

// NewV3Handler creates a new v3 handler.
func NewV3Handler() *V3Handler {
	return &V3Handler{
		facade:       runtimeapi.NewFacade(),
		execSessions: make(map[string]*localExecSession),
		pullOps:      make(map[string]*imagePullOperation),
		deployOps:    make(map[string]*deployApplyOperation),
	}
}

var localAPIUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

type localExecSession struct {
	SessionID        string
	MicroserviceUUID string
	callback         *localExecCallback
	createdAt        time.Time
}

type localExecCallback struct {
	stdinR  *io.PipeReader
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	stdoutW *io.PipeWriter
	stderrR *io.PipeReader
	stderrW *io.PipeWriter
	done    chan struct{}
	mu      sync.Mutex
	closed  bool
	err     error
}

type imagePullOperation struct {
	OperationID    string
	Status         string
	Progress       int
	Image          string
	ResolvedImage  string
	RegistryID     *int
	Platform       string
	Engine         string
	Error          string
	StartedAt      time.Time
	EndedAt        *time.Time
}

type deployApplyOperation struct {
	OperationID   string
	Status        string
	Stage         string
	Kind          string
	Name          string
	Image         string
	DeploymentID  string
	ErrorCode     string
	ErrorMessage  string
	StartedAt     time.Time
	EndedAt       *time.Time
}

func newLocalExecCallback() *localExecCallback {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	return &localExecCallback{
		stdinR:  stdinR,
		stdinW:  stdinW,
		stdoutR: stdoutR,
		stdoutW: stdoutW,
		stderrR: stderrR,
		stderrW: stderrW,
		done:    make(chan struct{}),
	}
}

func (c *localExecCallback) GetStdinReader() io.Reader  { return c.stdinR }
func (c *localExecCallback) GetStdoutWriter() io.Writer { return c.stdoutW }
func (c *localExecCallback) GetStderrWriter() io.Writer { return c.stderrW }
func (c *localExecCallback) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}
func (c *localExecCallback) OnComplete() { c.close(nil) }
func (c *localExecCallback) OnError(err error) {
	c.close(err)
}
func (c *localExecCallback) close(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.err = err
	_ = c.stdinW.Close()
	_ = c.stdoutW.Close()
	_ = c.stderrW.Close()
	close(c.done)
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
	if status, _ := result["status"].(string); strings.EqualFold(status, "failed") {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, fmt.Sprintf("%v", result["message"]), nil)
		return
	}
	writeSuccess(w, http.StatusOK, result)
}

func (h *V3Handler) HandleImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	if body, _ := io.ReadAll(r.Body); len(bytes.TrimSpace(body)) > 0 {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "request body is not allowed", nil)
		return
	}
	items, err := h.facade.ListImages()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"count": len(items),
	})
}

func (h *V3Handler) HandleImagePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Image      string `json:"image"`
		RegistryID *int   `json:"registryId,omitempty"`
		Platform   string `json:"platform,omitempty"`
		Async      bool   `json:"async,omitempty"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Image) == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "image is required", nil)
		return
	}
	if req.Async {
		op := &imagePullOperation{
			OperationID: uuid.NewString(),
			Status:      "running",
			Progress:    0,
			Image:       strings.TrimSpace(req.Image),
			RegistryID:  req.RegistryID,
			Platform:    strings.TrimSpace(req.Platform),
			Engine:      strings.ToLower(strings.TrimSpace(config.GetInstance().ContainerEngine)),
			StartedAt:   time.Now().UTC(),
		}
		h.pullMu.Lock()
		h.pullOps[op.OperationID] = op
		h.pullMu.Unlock()

		go func(operationID string) {
			resolvedImage, err := h.facade.PullImageWithProgress(req.Image, req.RegistryID, req.Platform, func(progress float32) {
				h.pullMu.Lock()
				defer h.pullMu.Unlock()
				current, ok := h.pullOps[operationID]
				if !ok {
					return
				}
				if progress < 0 {
					progress = 0
				}
				if progress > 100 {
					progress = 100
				}
				if int(progress) > current.Progress {
					current.Progress = int(progress)
				}
			})
			h.pullMu.Lock()
			defer h.pullMu.Unlock()
			current, ok := h.pullOps[operationID]
			if !ok {
				return
			}
			now := time.Now().UTC()
			current.EndedAt = &now
			current.ResolvedImage = strings.TrimSpace(resolvedImage)
			if err != nil {
				current.Status = "failed"
				current.Error = err.Error()
				return
			}
			current.Progress = 100
			current.Status = "succeeded"
		}(op.OperationID)

		writeSuccess(w, http.StatusAccepted, map[string]interface{}{
			"operationId": op.OperationID,
			"status":      op.Status,
			"progress":    op.Progress,
			"image":       op.Image,
			"registryId":  op.RegistryID,
			"platform":    op.Platform,
			"engine":      op.Engine,
			"startedAt":   op.StartedAt.Format(time.RFC3339Nano),
		})
		return
	}

	resolvedImage, err := h.facade.PullImage(req.Image, req.RegistryID, req.Platform)
	if err != nil {
		if strings.Contains(err.Error(), "registryId") || strings.Contains(err.Error(), "platform") || strings.Contains(err.Error(), "required") {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	payload := map[string]interface{}{
		"status":        "ok",
		"image":         strings.TrimSpace(req.Image),
		"resolvedImage": strings.TrimSpace(resolvedImage),
		"platform":      strings.TrimSpace(req.Platform),
		"engine":        strings.ToLower(strings.TrimSpace(config.GetInstance().ContainerEngine)),
		"message":       "image pulled successfully",
	}
	if req.RegistryID != nil {
		payload["registryId"] = *req.RegistryID
	}
	writeSuccess(w, http.StatusOK, payload)
}

func (h *V3Handler) HandleImagePullStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	operationID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v3/images:pull/"))
	if operationID == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing operation id", nil)
		return
	}
	h.pullMu.RLock()
	op, ok := h.pullOps[operationID]
	h.pullMu.RUnlock()
	if !ok {
		writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "pull operation not found", nil)
		return
	}
	response := map[string]interface{}{
		"operationId":   op.OperationID,
		"status":        op.Status,
		"progress":      op.Progress,
		"image":         op.Image,
		"resolvedImage": op.ResolvedImage,
		"platform":      op.Platform,
		"engine":        op.Engine,
		"startedAt":     op.StartedAt.Format(time.RFC3339Nano),
	}
	if op.RegistryID != nil {
		response["registryId"] = *op.RegistryID
	}
	if op.Error != "" {
		response["error"] = op.Error
	}
	if op.EndedAt != nil {
		response["endedAt"] = op.EndedAt.Format(time.RFC3339Nano)
	}
	writeSuccess(w, http.StatusOK, response)
}

func (h *V3Handler) HandleImageLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "path is required", nil)
		return
	}
	loaded, err := h.facade.LoadImageFromPath(req.Path)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	items := make([]map[string]interface{}, 0, len(loaded))
	for _, item := range loaded {
		items = append(items, map[string]interface{}{
			"name": item.Name,
			"id":   item.ID,
		})
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"loaded":  items,
		"count":   len(items),
		"engine":  strings.ToLower(strings.TrimSpace(config.GetInstance().ContainerEngine)),
		"message": "image archive loaded successfully",
	})
}

func (h *V3Handler) HandleImagePrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	result := h.facade.Prune()
	if status, _ := result["status"].(string); strings.EqualFold(status, "failed") {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, fmt.Sprintf("%v", result["message"]), nil)
		return
	}
	writeSuccess(w, http.StatusOK, result)
}

func (h *V3Handler) HandleImageRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		Selector string `json:"selector"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Selector) == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "selector is required", nil)
		return
	}
	removed, err := h.facade.RemoveImage(req.Selector)
	if err != nil {
		if strings.Contains(err.Error(), "ambiguous") || strings.Contains(err.Error(), "required") {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, err.Error(), nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"selector": strings.TrimSpace(req.Selector),
		"removed":  removed,
		"engine":   strings.ToLower(strings.TrimSpace(config.GetInstance().ContainerEngine)),
		"message":  "image removed successfully",
	})
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
		source := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
		if source == "" {
			source = "all"
		}
		if source != "all" && source != "managed" && source != "local" {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "source must be one of managed|local|all", nil)
			return
		}
		items := h.facade.ListRuntimeMicroservices()
		filtered := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			itemType := strings.ToLower(fmt.Sprintf("%v", item["type"]))
			if source == "all" || source == itemType {
				filtered = append(filtered, item)
			}
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"items": filtered,
		})
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/v3/ms/")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing microservice id", nil)
		return
	}
	parts := strings.Split(rest, "/")
	id := strings.TrimSpace(parts[0])
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing microservice id", nil)
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			item, err := h.facade.GetRuntimeMicroservice(id)
			if err != nil {
				writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, err.Error(), nil)
				return
			}
			if summary := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("summary")), "true"); summary {
				writeSuccess(w, http.StatusOK, map[string]interface{}{
					"uuid":         item["uuid"],
					"name":         item["name"],
					"application":  item["application"],
					"source":       item["source"],
					"type":         item["type"],
					"state":        item["state"],
					"containerId":  item["containerId"],
					"image":        item["image"],
					"healthStatus": item["healthStatus"],
				})
				return
			}
			writeSuccess(w, http.StatusOK, item)
		case http.MethodDelete:
			uuid, err := h.facade.RemoveRuntimeMicroservice(id)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
				return
			}
			writeSuccess(w, http.StatusOK, map[string]interface{}{
				"status":           "ok",
				"microserviceUuid": uuid,
				"warning":          "if microservice is controller-managed, reconcile may recreate it",
			})
		default:
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		}
		return
	}

	action := strings.TrimSpace(strings.Join(parts[1:], "/"))
	switch action {
	case "start":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		uuid, err := h.facade.StartRuntimeMicroservice(id)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"status":           "ok",
			"microserviceUuid": uuid,
			"warning":          "if microservice is controller-managed, reconcile may recreate or restart it",
		})
	case "stop":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		uuid, err := h.facade.StopRuntimeMicroservice(id)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"status":           "ok",
			"microserviceUuid": uuid,
			"warning":          "if microservice is controller-managed, reconcile may recreate or restart it",
		})
	case "restart":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		uuid, err := h.facade.RestartRuntimeMicroservice(id)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"status":           "ok",
			"microserviceUuid": uuid,
			"warning":          "if microservice is controller-managed, reconcile may recreate or restart it",
		})
	case "kill":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		uuid, err := h.facade.KillRuntimeMicroservice(id)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"status":           "ok",
			"microserviceUuid": uuid,
			"warning":          "if microservice is controller-managed, reconcile may recreate or restart it",
		})
	case "logs":
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		tailLines := 100
		tailRaw := strings.TrimSpace(r.URL.Query().Get("tailLines"))
		if tailRaw == "" {
			tailRaw = strings.TrimSpace(r.URL.Query().Get("tail"))
		}
		if tailRaw != "" {
			parsed, err := strconv.Atoi(tailRaw)
			if err != nil || parsed <= 0 {
				writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "tailLines must be a positive integer", nil)
				return
			}
			tailLines = parsed
		}
		uuid, entries, err := h.facade.GetRuntimeMicroserviceLogs(id, tailLines, strings.TrimSpace(r.URL.Query().Get("since")), strings.TrimSpace(r.URL.Query().Get("until")))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{
			"microserviceUuid": uuid,
			"entries":          entries,
		})
	case "logs:stream":
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		h.handleLogsStreamWS(w, r, id)
	case "exec/sessions":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		h.handleCreateExecSession(w, r, id)
	default:
		if strings.HasPrefix(action, "exec/sessions/") {
			sessionPart := strings.TrimPrefix(action, "exec/sessions/")
			if strings.HasSuffix(sessionPart, ":attach") {
				if r.Method != http.MethodGet {
					writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
					return
				}
				sessionID := strings.TrimSuffix(sessionPart, ":attach")
				h.handleAttachExecSessionWS(w, r, id, sessionID)
				return
			}
			sessionID := strings.TrimSpace(sessionPart)
			if sessionID == "" {
				writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing exec session id", nil)
				return
			}
			switch r.Method {
			case http.MethodGet:
				h.handleGetExecSessionStatus(w, id, sessionID)
			case http.MethodDelete:
				h.handleStopExecSession(w, id, sessionID)
			default:
				writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			}
			return
		}
		writeAPIError(w, http.StatusNotImplemented, ErrCodeNotImplemented, "not implemented", nil)
	}
}

func (h *V3Handler) HandleDeployMicroservicesApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	manifest, sourceName, dryRun, err := parseManifestMultipartRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	asyncRaw := strings.TrimSpace(strings.ToLower(r.FormValue("async")))
	switch asyncRaw {
	case "", "false", "0", "no", "true", "1", "yes":
	default:
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "async must be a boolean", nil)
		return
	}

	manifestDoc, err := h.facade.ParseAndValidateLocalManifest(manifest)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	op := &deployApplyOperation{
		OperationID: uuid.NewString(),
		Status:      "running",
		Stage:       runtimeapi.DeployStageParsing,
		Kind:        strings.TrimSpace(manifestDoc.Kind),
		Name:        strings.TrimSpace(manifestDoc.Metadata.Name),
		Image:       strings.TrimSpace(manifestDoc.ResolveImageForArch(facadePlatformArch())),
		StartedAt:   time.Now().UTC(),
	}

	h.deployMu.Lock()
	h.deployOps[op.OperationID] = op
	h.deployMu.Unlock()

	go func(operationID string, manifestText string, source string, applyDryRun bool) {
		deploymentID, _, applyErr := h.facade.ApplyLocalManifest(manifestText, source, applyDryRun, func(stage string, _ string) {
			normalized := strings.TrimSpace(strings.ToLower(stage))
			if normalized == "" {
				return
			}
			h.deployMu.Lock()
			defer h.deployMu.Unlock()
			current, ok := h.deployOps[operationID]
			if !ok {
				return
			}
			current.Stage = normalized
		})

		h.deployMu.Lock()
		defer h.deployMu.Unlock()
		current, ok := h.deployOps[operationID]
		if !ok {
			return
		}
		now := time.Now().UTC()
		current.EndedAt = &now
		if applyErr != nil {
			current.Status = "failed"
			current.ErrorCode = ErrCodeInternal
			current.ErrorMessage = applyErr.Error()
			return
		}
		current.Status = "succeeded"
		current.Stage = runtimeapi.DeployStageDone
		current.DeploymentID = strings.TrimSpace(deploymentID)
	}(op.OperationID, manifest, sourceName, dryRun)

	writeSuccess(w, http.StatusAccepted, map[string]interface{}{
		"operationId": op.OperationID,
		"status":      op.Status,
		"kind":        op.Kind,
		"name":        op.Name,
		"image":       op.Image,
		"startedAt":   op.StartedAt.Format(time.RFC3339Nano),
	})
}

func (h *V3Handler) HandleDeployMicroservicesApplyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	operationID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v3/deploy/microservices:apply/"))
	if operationID == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing operation id", nil)
		return
	}
	h.deployMu.RLock()
	op, ok := h.deployOps[operationID]
	h.deployMu.RUnlock()
	if !ok {
		writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "deploy apply operation not found", nil)
		return
	}
	response := map[string]interface{}{
		"operationId": op.OperationID,
		"status":      op.Status,
		"startedAt":   op.StartedAt.Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(op.Stage) != "" {
		response["stage"] = op.Stage
	}
	if strings.TrimSpace(op.Kind) != "" {
		response["kind"] = op.Kind
	}
	if strings.TrimSpace(op.Name) != "" {
		response["name"] = op.Name
	}
	if strings.TrimSpace(op.Image) != "" {
		response["image"] = op.Image
	}
	if strings.TrimSpace(op.DeploymentID) != "" {
		response["deploymentId"] = op.DeploymentID
	}
	if op.EndedAt != nil {
		response["endedAt"] = op.EndedAt.Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(op.ErrorMessage) != "" {
		code := strings.TrimSpace(op.ErrorCode)
		if code == "" {
			code = ErrCodeInternal
		}
		response["error"] = map[string]interface{}{
			"code":    code,
			"message": op.ErrorMessage,
		}
	}
	writeSuccess(w, http.StatusOK, response)
}

func (h *V3Handler) HandleDeployMicroservicesValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	manifest, _, _, err := parseManifestMultipartRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	doc, err := h.facade.ParseAndValidateLocalManifest(manifest)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"valid":      true,
		"kind":       doc.Kind,
		"name":       doc.Metadata.Name,
		"apiVersion": doc.APIVersion,
		"image":      doc.ResolveImageForArch(facadePlatformArch()),
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
	manifest, _, dryRun, err := parseManifestMultipartRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	reg, err := h.facade.ApplyLocalRegistryManifest(manifest, dryRun)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"accepted": true,
		"dryRun":   dryRun,
		"registry": reg,
	})
}

func (h *V3Handler) HandleDeployRegistriesValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	manifest, _, _, err := parseManifestMultipartRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	doc, err := h.facade.ParseAndValidateLocalRegistryManifest(manifest)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"valid":      true,
		"apiVersion": doc.APIVersion,
		"kind":       doc.Kind,
		"url":        doc.Spec.URL,
		"private":    doc.Spec.Private,
	})
}

func (h *V3Handler) HandleDeployRegistries(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v3/deploy/registries" {
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
		return
	}

	idRaw := strings.TrimPrefix(r.URL.Path, "/v3/deploy/registries/")
	idRaw = strings.TrimSpace(idRaw)
	id, err := strconv.Atoi(idRaw)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid registry id", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := h.facade.GetRegistry(id)
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
		if err := h.facade.DeleteRegistry(id); err != nil {
			writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
			return
		}
		writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "ok", "id": id})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
	}
}

func parseManifestMultipartRequest(r *http.Request) (manifest, sourceName string, dryRun bool, err error) {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		return "", "", false, fmt.Errorf("invalid multipart form body")
	}
	file, _, ferr := r.FormFile("manifest")
	if ferr != nil {
		return "", "", false, fmt.Errorf("manifest file part is required")
	}
	defer file.Close()
	raw, rerr := io.ReadAll(file)
	if rerr != nil {
		return "", "", false, fmt.Errorf("failed to read manifest")
	}
	manifest = strings.TrimSpace(string(raw))
	if manifest == "" {
		return "", "", false, fmt.Errorf("manifest file is empty")
	}
	sourceName = strings.TrimSpace(r.FormValue("sourceName"))
	dryRaw := strings.TrimSpace(strings.ToLower(r.FormValue("dryRun")))
	switch dryRaw {
	case "", "false", "0", "no":
		dryRun = false
	case "true", "1", "yes":
		dryRun = true
	default:
		return "", "", false, fmt.Errorf("dryRun must be a boolean")
	}
	return manifest, sourceName, dryRun, nil
}

func facadePlatformArch() string {
	cfg := config.GetInstance()
	if cfg == nil {
		return "x86"
	}
	arch := strings.TrimSpace(strings.ToLower(cfg.Arch))
	switch arch {
	case "arm", "arm64", "aarch64":
		return "arm"
	default:
		return "x86"
	}
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

func (h *V3Handler) handleCreateExecSession(w http.ResponseWriter, r *http.Request, selector string) {
	var req struct {
		Command []string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "invalid JSON body", nil)
		return
	}
	if len(req.Command) == 0 {
		req.Command = []string{
			"/bin/sh", "-lc",
			"if [ -x /bin/bash ]; then exec /bin/bash; elif [ -x /bin/sh ]; then exec /bin/sh; else exec /busybox/sh; fi",
		}
	}
	if len(req.Command) == 1 && strings.TrimSpace(req.Command[0]) == "" {
		req.Command = []string{
			"/bin/sh", "-lc",
			"if [ -x /bin/bash ]; then exec /bin/bash; elif [ -x /bin/sh ]; then exec /bin/sh; else exec /busybox/sh; fi",
		}
	}
	uuid, err := h.facade.ResolveMicroserviceID(selector)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	callback := newLocalExecCallback()
	execID, err := processmanager.GetInstance().CreateExecSession(uuid, req.Command, callback)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	h.execMu.Lock()
	h.execSessions[execID] = &localExecSession{
		SessionID:        execID,
		MicroserviceUUID: uuid,
		callback:         callback,
		createdAt:        time.Now().UTC(),
	}
	h.execMu.Unlock()
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"sessionId": execID,
		"wsUrl":     fmt.Sprintf("/v3/ms/%s/exec/sessions/%s:attach", selector, execID),
	})
}

func (h *V3Handler) handleGetExecSessionStatus(w http.ResponseWriter, selector, sessionID string) {
	uuid, err := h.facade.ResolveMicroserviceID(selector)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	h.execMu.RLock()
	session, ok := h.execSessions[sessionID]
	h.execMu.RUnlock()
	if !ok || session.MicroserviceUUID != uuid {
		writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "exec session not found", nil)
		return
	}
	running, err := processmanager.GetInstance().GetExecSessionStatus(sessionID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"sessionId":        sessionID,
		"microserviceUuid": uuid,
		"running":          running,
		"exitCode": func() interface{} {
			code, codeErr := processmanager.GetInstance().GetExecSessionExitCode(sessionID)
			if codeErr != nil {
				return nil
			}
			return code
		}(),
	})
}

func (h *V3Handler) handleStopExecSession(w http.ResponseWriter, selector, sessionID string) {
	uuid, err := h.facade.ResolveMicroserviceID(selector)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	if err := processmanager.GetInstance().StopExecSession(uuid, sessionID); err != nil {
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	h.execMu.Lock()
	delete(h.execSessions, sessionID)
	h.execMu.Unlock()
	writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "ok", "sessionId": sessionID})
}

func (h *V3Handler) handleAttachExecSessionWS(w http.ResponseWriter, r *http.Request, selector, sessionID string) {
	uuid, err := h.facade.ResolveMicroserviceID(selector)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	h.execMu.RLock()
	session, ok := h.execSessions[sessionID]
	h.execMu.RUnlock()
	if !ok || session.MicroserviceUUID != uuid {
		writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "exec session not found", nil)
		return
	}
	conn, err := localAPIUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	var writeMu sync.Mutex
	wg.Add(3)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := session.callback.stdoutR.Read(buf)
			if n > 0 {
				writeMu.Lock()
				writeErr := conn.WriteJSON(map[string]interface{}{"stream": "stdout", "line": string(buf[:n])})
				writeMu.Unlock()
				if writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := session.callback.stderrR.Read(buf)
			if n > 0 {
				writeMu.Lock()
				writeErr := conn.WriteJSON(map[string]interface{}{"stream": "stderr", "line": string(buf[:n])})
				writeMu.Unlock()
				if writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			_, msg, readErr := conn.ReadMessage()
			if readErr != nil {
				_ = session.callback.stdinW.Close()
				return
			}
			var ctrl struct {
				Type string `json:"type"`
				Cols uint32 `json:"cols"`
				Rows uint32 `json:"rows"`
			}
			if json.Unmarshal(msg, &ctrl) == nil && strings.EqualFold(strings.TrimSpace(ctrl.Type), "resize") {
				_ = processmanager.GetInstance().ResizeExecSession(sessionID, ctrl.Cols, ctrl.Rows)
				continue
			}
			_, _ = session.callback.stdinW.Write(msg)
		}
	}()
	select {
	case <-session.callback.done:
	case <-time.After(10 * time.Minute):
	}
	exitCode, _ := processmanager.GetInstance().GetExecSessionExitCode(sessionID)
	_ = processmanager.GetInstance().StopExecSession(uuid, sessionID)
	h.execMu.Lock()
	delete(h.execSessions, sessionID)
	h.execMu.Unlock()
	writeMu.Lock()
	_ = conn.WriteJSON(map[string]interface{}{
		"stream":   "control",
		"line":     "session closed",
		"exitCode": exitCode,
	})
	writeMu.Unlock()
	_ = conn.Close()
	wg.Wait()
}

func (h *V3Handler) handleLogsStreamWS(w http.ResponseWriter, r *http.Request, selector string) {
	conn, err := localAPIUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	uuid, err := h.facade.ResolveMicroserviceID(selector)
	if err != nil {
		_ = conn.WriteJSON(map[string]interface{}{"error": err.Error()})
		return
	}
	seen := make(map[string]struct{})
	for i := 0; i < 300; i++ {
		_, entries, logsErr := h.facade.GetRuntimeMicroserviceLogs(uuid, 50, "", "")
		if logsErr != nil {
			_ = conn.WriteJSON(map[string]interface{}{"error": logsErr.Error()})
			return
		}
		for _, entry := range entries {
			key := fmt.Sprintf("%v|%v|%v", entry["ts"], entry["stream"], entry["line"])
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			_ = conn.WriteJSON(entry)
		}
		time.Sleep(1 * time.Second)
	}
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
