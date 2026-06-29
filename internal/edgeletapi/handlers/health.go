package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
)

var (
	agentStartTime = time.Now()
	localAPIState  = edgeletAPIStartupState{
		phase:  EdgeletAPIStartupInitializing,
		reason: "local_api_initializing",
	}
	localAPIStateMu sync.RWMutex
)

const (
	EdgeletAPIStartupInitializing = "initializing"
	EdgeletAPIStartupListening    = "listening"
	EdgeletAPIStartupFailed       = "failed"
)

type edgeletAPIStartupState struct {
	phase  string
	reason string
}

// SetEdgeletAPIStartupState updates the listener startup phase for health checks.
func SetEdgeletAPIStartupState(phase, reason string) {
	localAPIStateMu.Lock()
	defer localAPIStateMu.Unlock()
	localAPIState.phase = strings.TrimSpace(phase)
	localAPIState.reason = strings.TrimSpace(reason)
}

func getEdgeletAPIStartupState() edgeletAPIStartupState {
	localAPIStateMu.RLock()
	defer localAPIStateMu.RUnlock()
	return localAPIState
}

// HealthLiveHandler handles /health/live — liveness probe.
// Returns 200 if the process is running and the HTTP server can respond.
// Used by orchestrators (Kubernetes, systemd) to determine if the process should be restarted.
func HealthLiveHandler(w http.ResponseWriter, _ *http.Request) {
	state := getEdgeletAPIStartupState()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"ok","localApiPhase":"%s"}`, state.phase)
}

// HealthReadyHandler handles /health/ready — readiness probe.
// Returns 200 when the local API is listening and the supervisor has finished engine wiring.
// Controller connectivity is reported separately in status; it does not gate local readiness.
func HealthReadyHandler(w http.ResponseWriter, _ *http.Request) {
	state := getEdgeletAPIStartupState()
	if state.phase == EdgeletAPIStartupFailed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"status":"not_ready","reason":"local_api_start_failed","detail":"%s"}`, state.reason)
		return
	}
	if state.phase != EdgeletAPIStartupListening {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"status":"not_ready","reason":"local_api_listener_not_ready","phase":"%s"}`, state.phase)
		return
	}

	cfg := config.GetInstance()
	if cfg == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready","reason":"config_not_loaded"}`))
		return
	}

	sr := statusreporter.GetInstance()
	if sr != nil && sr.DaemonOperational() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"status":"not_ready","reason":"daemon_not_running"}`))
}

// MetricsStartTime returns agent start time for metrics.
func MetricsStartTime() time.Time {
	return agentStartTime
}
