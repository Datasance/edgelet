package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/runtimeapi"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/google/uuid"
)

type controlPlaneApplyOperation struct {
	OperationID    string
	Status         string
	Stage          string
	ControllerUUID string
	Generation     int64
	Namespace      string
	Name           string
	Image          string
	Mode           string
	ContainerID    string
	RuntimeState   string
	ErrorCode      string
	ErrorMessage   string
	StartedAt      time.Time
	EndedAt        *time.Time
}

// HandleSystemControlPlane routes GET/DELETE /v1/system/controlplane and GET manifest.
func (h *EdgeletAPIHandler) HandleSystemControlPlane(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v1/system/controlplane/manifest":
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
			return
		}
		h.handleSystemControlPlaneManifest(w)
	case r.URL.Path == "/v1/system/controlplane":
		switch r.Method {
		case http.MethodGet:
			h.handleSystemControlPlaneStatus(w)
		case http.MethodDelete:
			h.handleSystemControlPlaneDelete(w)
		default:
			writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		}
	default:
		writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "not found", nil)
	}
}

// HandleSystemControllerStatus is the GET /v1/system/controller status alias.
func (h *EdgeletAPIHandler) HandleSystemControllerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	h.handleSystemControlPlaneStatus(w)
}

func (h *EdgeletAPIHandler) handleSystemControlPlaneStatus(w http.ResponseWriter) {
	item, err := h.facade.GetControlPlaneDeployment()
	if err != nil {
		if errors.Is(err, runtimeapi.ErrControlPlaneNotFound) {
			writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "control plane deployment not found", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, runtimeapi.ControlPlaneStatusMap(item))
}

func (h *EdgeletAPIHandler) handleSystemControlPlaneManifest(w http.ResponseWriter) {
	masked, err := h.facade.GetControlPlaneManifestMasked()
	if err != nil {
		if errors.Is(err, runtimeapi.ErrControlPlaneNotFound) {
			writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "control plane deployment not found", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"manifestYaml": masked,
		"masked":       true,
	})
}

func (h *EdgeletAPIHandler) handleSystemControlPlaneDelete(w http.ResponseWriter) {
	if err := h.facade.DeleteControlPlane(); err != nil {
		if errors.Is(err, runtimeapi.ErrControlPlaneNotFound) {
			writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "control plane deployment not found", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, ErrCodeInternal, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (h *EdgeletAPIHandler) HandleDeployControlPlaneApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	manifest, sourceName, dryRun, err := parseManifestMultipartRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	if activeID, busy := h.activeControlPlaneApply(); busy {
		writeAPIError(w, http.StatusConflict, ErrCodeApplyInProgress, "control plane apply already in progress", map[string]interface{}{
			"activeOperationId": activeID,
		})
		return
	}

	doc, err := h.facade.ParseAndValidateControlPlaneManifest(manifest)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	image := doc.ManifestControllerImage()
	op := &controlPlaneApplyOperation{
		OperationID: uuid.NewString(),
		Status:      "running",
		Stage:       runtimeapi.DeployStageParsing,
		Namespace:   strings.TrimSpace(doc.Metadata.Namespace),
		Name:        strings.TrimSpace(doc.Metadata.Name),
		Image:       image,
		StartedAt:   time.Now().UTC(),
	}

	h.controlPlaneApplyMu.Lock()
	h.controlPlaneApplyOps[op.OperationID] = op
	h.controlPlaneApplyMu.Unlock()
	logging.LogInfo(apiHandlerModuleName, fmt.Sprintf("control plane apply operation started operationId=%s source=%s dryRun=%v namespace=%s name=%s", op.OperationID, strings.TrimSpace(sourceName), dryRun, op.Namespace, op.Name))

	go func(operationID, manifestText, source string, applyDryRun bool) {
		result, applyErr := h.facade.ApplyControlPlaneManifest(manifestText, source, applyDryRun, func(stage string, _ string) {
			normalized := strings.TrimSpace(strings.ToLower(stage))
			if normalized == "" {
				return
			}
			h.controlPlaneApplyMu.Lock()
			current, ok := h.controlPlaneApplyOps[operationID]
			if ok {
				current.Stage = normalized
			}
			h.controlPlaneApplyMu.Unlock()
		})

		h.controlPlaneApplyMu.Lock()
		defer h.controlPlaneApplyMu.Unlock()
		current, ok := h.controlPlaneApplyOps[operationID]
		if !ok {
			return
		}
		now := time.Now().UTC()
		current.EndedAt = &now
		if applyErr != nil {
			current.Status = "failed"
			var identityErr *runtimeapi.ErrControlPlaneIdentityImmutable
			if errors.As(applyErr, &identityErr) {
				current.ErrorCode = ErrCodeConflict
			} else {
				current.ErrorCode = ErrCodeInvalidArgument
			}
			current.ErrorMessage = applyErr.Error()
			logging.LogWarn(apiHandlerModuleName, fmt.Sprintf("control plane apply failed operationId=%s dryRun=%v err=%v", operationID, applyDryRun, applyErr))
			return
		}
		current.Status = "succeeded"
		current.Stage = runtimeapi.DeployStageDone
		current.Mode = result.Mode
		current.ControllerUUID = result.ControllerUUID
		current.Generation = result.Generation
		current.ContainerID = result.ContainerID
		current.RuntimeState = result.RuntimeState
		logging.LogInfo(apiHandlerModuleName, fmt.Sprintf("control plane apply succeeded operationId=%s controllerUuid=%s dryRun=%v", operationID, result.ControllerUUID, applyDryRun))
	}(op.OperationID, manifest, sourceName, dryRun)

	writeSuccess(w, http.StatusAccepted, map[string]interface{}{
		"operationId":    op.OperationID,
		"status":         op.Status,
		"controllerUuid": op.ControllerUUID,
		"generation":     op.Generation,
		"namespace":      op.Namespace,
		"name":           op.Name,
		"image":          op.Image,
		"startedAt":      op.StartedAt.Format(time.RFC3339Nano),
	})
}

func (h *EdgeletAPIHandler) HandleDeployControlPlaneApplyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	operationID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/deploy/controlplane:apply/"))
	if operationID == "" {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, "missing operation id", nil)
		return
	}
	h.controlPlaneApplyMu.RLock()
	op, ok := h.controlPlaneApplyOps[operationID]
	h.controlPlaneApplyMu.RUnlock()
	if !ok {
		writeAPIError(w, http.StatusNotFound, ErrCodeNotFound, "control plane apply operation not found", nil)
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
	if strings.TrimSpace(op.ControllerUUID) != "" {
		response["controllerUuid"] = op.ControllerUUID
	}
	if op.Generation > 0 {
		response["generation"] = op.Generation
	}
	if strings.TrimSpace(op.Namespace) != "" {
		response["namespace"] = op.Namespace
	}
	if strings.TrimSpace(op.Name) != "" {
		response["name"] = op.Name
	}
	if strings.TrimSpace(op.Image) != "" {
		response["image"] = op.Image
	}
	if strings.TrimSpace(op.Mode) != "" {
		response["mode"] = op.Mode
	}
	if strings.TrimSpace(op.ContainerID) != "" {
		response["containerId"] = op.ContainerID
	}
	if strings.TrimSpace(op.RuntimeState) != "" {
		response["runtimeState"] = op.RuntimeState
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

func (h *EdgeletAPIHandler) activeControlPlaneApply() (string, bool) {
	h.controlPlaneApplyMu.RLock()
	defer h.controlPlaneApplyMu.RUnlock()
	for id, op := range h.controlPlaneApplyOps {
		if op != nil && strings.EqualFold(strings.TrimSpace(op.Status), "running") {
			return id, true
		}
	}
	return "", false
}

func (h *EdgeletAPIHandler) HandleDeployControlPlaneValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "method not allowed", nil)
		return
	}
	manifest, _, _, err := parseManifestMultipartRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	doc, err := h.facade.ParseAndValidateControlPlaneManifest(manifest)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
		return
	}
	writeSuccess(w, http.StatusOK, map[string]interface{}{
		"valid":      true,
		"kind":       doc.Kind,
		"name":       doc.Metadata.Name,
		"namespace":  doc.Metadata.Namespace,
		"apiVersion": doc.APIVersion,
		"image":      doc.ManifestControllerImage(),
	})
}
