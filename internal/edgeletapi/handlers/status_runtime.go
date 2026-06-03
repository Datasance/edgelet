package handlers

import (
	"strconv"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/runtime"
)

func augmentWithRuntimeStatus(status map[string]string) {
	if status == nil {
		return
	}
	cfg := config.GetInstance()
	rs := runtime.GetState()
	status["runtime.engine"] = cfg.ContainerEngine
	status["runtime.dockerUrl"] = cfg.DockerURL
	status["runtime.pendingRestart"] = strconv.FormatBool(rs.PendingRestart())
	status["runtime.engineReady"] = strconv.FormatBool(rs.EngineReady())
	status["runtime.shutdownPolicy"] = cfg.ShutdownPolicy
	if phase := rs.AgentPhase(); phase != "" {
		status["runtime.agentPhase"] = phase
	}
}
