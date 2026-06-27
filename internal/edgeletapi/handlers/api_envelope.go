package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/eclipse-iofog/edgelet/internal/runtimeapi"
)

const (
	// Stable v3 error code taxonomy.
	ErrCodeInvalidArgument  = "INVALID_ARGUMENT"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeForbidden        = "FORBIDDEN"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeConflict         = "CONFLICT"
	ErrCodeApplyInProgress  = "APPLY_IN_PROGRESS"
	ErrCodeNotImplemented   = "NOT_IMPLEMENTED"
	ErrCodeInternal         = "INTERNAL"
	ErrCodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	ErrCodeExecStartTimeout = "EXEC_START_TIMEOUT"
)

type apiErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
}

type apiErrorEnvelope struct {
	Success bool         `json:"success"`
	Error   apiErrorBody `json:"error"`
}

type apiSuccessEnvelope struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
}

func writeSuccess(w http.ResponseWriter, statusCode int, payload any) {
	writeJSONEnvelope(w, statusCode, apiSuccessEnvelope{
		Success: true,
		Data:    payload,
	})
}

func writeMicroserviceLifecycleError(w http.ResponseWriter, err error) {
	if runtimeapi.IsControlPlaneLifecycleBlocked(err) {
		writeAPIError(w, http.StatusForbidden, ErrCodeForbidden, err.Error(), nil)
		return
	}
	writeAPIError(w, http.StatusBadRequest, ErrCodeInvalidArgument, err.Error(), nil)
}

func writeAPIError(w http.ResponseWriter, statusCode int, code, message string, details map[string]any) {
	writeJSONEnvelope(w, statusCode, apiErrorEnvelope{
		Success: false,
		Error: apiErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func writeJSONEnvelope(w http.ResponseWriter, statusCode int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}
