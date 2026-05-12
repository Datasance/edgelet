package handlers

import (
	"encoding/json"
	"net/http"
)

const (
	// Stable v3 error code taxonomy.
	ErrCodeInvalidArgument   = "INVALID_ARGUMENT"
	ErrCodeUnauthorized      = "UNAUTHORIZED"
	ErrCodeForbidden         = "FORBIDDEN"
	ErrCodeNotFound          = "NOT_FOUND"
	ErrCodeConflict          = "CONFLICT"
	ErrCodeNotImplemented    = "NOT_IMPLEMENTED"
	ErrCodeInternal          = "INTERNAL"
	ErrCodeMethodNotAllowed  = "METHOD_NOT_ALLOWED"
)

type apiErrorBody struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
}

type apiErrorEnvelope struct {
	Success bool         `json:"success"`
	Error   apiErrorBody `json:"error"`
}

type apiSuccessEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func writeSuccess(w http.ResponseWriter, statusCode int, payload interface{}) {
	writeJSONEnvelope(w, statusCode, apiSuccessEnvelope{
		Success: true,
		Data:    payload,
	})
}

func writeAPIError(w http.ResponseWriter, statusCode int, code, message string, details map[string]interface{}) {
	writeJSONEnvelope(w, statusCode, apiErrorEnvelope{
		Success: false,
		Error: apiErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func writeJSONEnvelope(w http.ResponseWriter, statusCode int, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}
