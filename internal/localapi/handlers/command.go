package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/eclipse-iofog/agent-go/internal/cli"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	commandHandlerModuleName = "CommandLine Api Handler"
)

// CommandHandler handles command line API requests
type CommandHandler struct {
	// Will be injected with CommandLineParser when CLI is implemented
}

// CommandRequest represents the request body
type CommandRequest struct {
	Command string `json:"command"`
}

// CommandResponse represents the response body
type CommandResponse struct {
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

// HandleCommandLine handles POST /v2/commandline
func (h *CommandHandler) HandleCommandLine(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(commandHandlerModuleName, "Handle commandline api http request")

	if r.Method != http.MethodPost {
		logging.LogError(commandHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		logging.LogError(commandHandlerModuleName, "Invalid content type: "+contentType, nil)
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.LogError(commandHandlerModuleName, "Failed to read request body", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON
	var cmdReq CommandRequest
	if err := json.Unmarshal(body, &cmdReq); err != nil {
		logging.LogError(commandHandlerModuleName, "Failed to parse JSON", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Log specific command
	logging.LogInfo(commandHandlerModuleName, "Processing command: "+cmdReq.Command)

	// Execute command using CLI parser
	result, err := cli.ParseCommand(cmdReq.Command)
	if err != nil {
		response := CommandResponse{
			Response: "",
			Error:    err.Error(),
		}
		jsonData, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(jsonData)
		return
	}

	response := CommandResponse{
		Response: result,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(commandHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
	logging.LogDebug(commandHandlerModuleName, "Finished processing commandline api request")
}
