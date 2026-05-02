package handlers

import (
	"net/http"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
)

var (
	agentStartTime = time.Now()
)

// HealthLiveHandler handles /health/live — liveness probe.
// Returns 200 if the process is running and the HTTP server can respond.
// Used by orchestrators (Kubernetes, systemd) to determine if the process should be restarted.
func HealthLiveHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// HealthReadyHandler handles /health/ready — readiness probe.
// Returns 200 if the agent is provisioned and ready to serve (e.g. modules started).
// Returns 503 if not yet ready (e.g. still starting, not provisioned).
func HealthReadyHandler(w http.ResponseWriter, _ *http.Request) {
	cfg := config.GetInstance()
	if cfg == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready","reason":"config_not_loaded"}`))
		return
	}

	// Provisioned and daemon running = ready
	if cfg.IOFogUUID != "" {
		sr := statusreporter.GetInstance()
		if sr != nil {
			ss := sr.GetSupervisorStatus()
			if ss != nil && ss.DaemonStatus == models.ModuleStatusRunning {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ready"}`))
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"status":"not_ready","reason":"not_provisioned_or_not_running"}`))
}

// MetricsStartTime returns agent start time for metrics.
func MetricsStartTime() time.Time {
	return agentStartTime
}
