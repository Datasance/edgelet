package runtimestate

import (
	"strings"
	"sync"

	"github.com/eclipse-iofog/edgelet/internal/constants"
)

// State tracks in-process engine lifecycle flags for operators and reload policy.
type State struct {
	mu sync.RWMutex

	startupEngine  string
	pendingRestart bool
	engineReady    bool
	agentPhase     string
}

var instance = &State{}

// GetState returns the process-wide runtime state singleton.
func GetState() *State {
	return instance
}

// ResetForTests clears runtime state (tests only).
func ResetForTests() {
	instance = &State{}
}

// RecordStartupEngine snapshots the active engine at supervisor start.
func (s *State) RecordStartupEngine(engine string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startupEngine = normalizeEngine(engine)
	s.pendingRestart = false
}

// StartupEngine returns the engine family active when the supervisor started.
func (s *State) StartupEngine() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.startupEngine
}

// SetPendingRestart marks that a containerEngine change requires service restart.
func (s *State) SetPendingRestart(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingRestart = v
}

// PendingRestart reports whether the operator must restart edgelet.service.
func (s *State) PendingRestart() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingRestart
}

// SetEngineReady records whether a container engine client is wired and usable.
func (s *State) SetEngineReady(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engineReady = v
}

// EngineReady reports whether the runtime engine client is active.
func (s *State) EngineReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engineReady
}

// SetAgentPhase records control-plane lifecycle phase for status (e.g. restarting).
func (s *State) SetAgentPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentPhase = strings.TrimSpace(phase)
}

// AgentPhase returns the current control-plane phase exposed on system status.
func (s *State) AgentPhase() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agentPhase
}

func normalizeEngine(engine string) string {
	switch engine {
	case constants.EngineDocker, constants.EnginePodman, constants.EngineEdgelet:
		return engine
	default:
		return constants.EngineEdgelet
	}
}
