package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
)

// MetricsHandler handles /metrics — Prometheus-format metrics.
// Exposes basic agent metrics for enterprise observability.
func MetricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var b []byte
	// Agent uptime in seconds
	uptime := time.Since(MetricsStartTime()).Seconds()
	b = append(b, "# HELP iofog_agent_uptime_seconds Agent process uptime in seconds\n"...)
	b = append(b, "# TYPE iofog_agent_uptime_seconds gauge\n"...)
	b = append(b, "iofog_agent_uptime_seconds "...)
	b = append(b, strconv.FormatFloat(uptime, 'f', -1, 64)...)
	b = append(b, "\n"...)

	// Provisioned (1 or 0)
	cfg := config.GetInstance()
	provisioned := 0
	if cfg != nil && cfg.IOFogUUID != "" {
		provisioned = 1
	}
	b = append(b, "# HELP iofog_agent_provisioned Whether the agent is provisioned (1) or not (0)\n"...)
	b = append(b, "# TYPE iofog_agent_provisioned gauge\n"...)
	b = append(b, "iofog_agent_provisioned "...)
	b = append(b, strconv.Itoa(provisioned)...)
	b = append(b, "\n"...)

	// Running microservices count
	sr := statusreporter.GetInstance()
	runningCount := 0
	if sr != nil {
		if pm := sr.GetProcessManagerStatus(); pm != nil {
			runningCount = pm.RunningMicroservicesCount
		}
	}
	b = append(b, "# HELP iofog_agent_running_microservices Number of running microservices\n"...)
	b = append(b, "# TYPE iofog_agent_running_microservices gauge\n"...)
	b = append(b, "iofog_agent_running_microservices "...)
	b = append(b, strconv.Itoa(runningCount)...)
	b = append(b, "\n"...)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
