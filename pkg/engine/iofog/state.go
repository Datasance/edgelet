//go:build linux

package iofog

import (
	"strconv"
	"sync"
	"time"
)

// Container label keys stored on each containerd container.
// All keys are prefixed "iofog-" to avoid collisions with containerd's own labels.
const (
	labelIP          = "iofog-ip"
	labelNetns       = "iofog-netns"
	labelSandboxID   = "iofog-sandbox-id"
	labelStartedAt   = "iofog-started-at"
	labelPorts       = "iofog-ports"
	labelLogSize     = "iofog-log-size"
	labelHealthcheck = "iofog-healthcheck"
	// labelHostNet is set to "true" on host-network containers. The OCI spec for CRI
	// containers always has a non-empty network namespace path (pointing to the sandbox
	// netns), so spec-based detection of host-network mode is unreliable.
	labelHostNet = "iofog-hostnet"
	// labelIOFogUUID is the fog node's own UUID. Used by StopRunningMicroservices to
	// filter containers belonging to this agent instance on deprovision.
	labelIOFogUUID = "iofog-uuid"
)

// containerState holds per-container runtime state kept in memory.
// Critical fields (ip, netnsPath, sandboxID) are also persisted in container labels so
// the state can be recovered after an agent restart.
type containerState struct {
	netnsPath     string
	ip            string
	sandboxID     string    // CRI pod sandbox ID; used for teardown
	startedAt     int64     // Unix milliseconds; 0 if unknown
	prevCPUTime   int64     // CPU usage in microseconds at last sample
	prevCPUSample time.Time // Wall-clock time of last CPU sample
}

// stateStore is a thread-safe in-memory map from containerID to containerState.
type stateStore struct {
	mu    sync.RWMutex
	items map[string]*containerState
}

func newStateStore() *stateStore {
	return &stateStore{items: make(map[string]*containerState)}
}

func (s *stateStore) get(id string) (*containerState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.items[id]
	return st, ok
}

func (s *stateStore) set(id string, st *containerState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = st
}

func (s *stateStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
}

// stateFromLabels reconstructs a containerState from containerd container labels.
func stateFromLabels(labels map[string]string) *containerState {
	st := &containerState{
		netnsPath: labels[labelNetns],
		ip:        labels[labelIP],
		sandboxID: labels[labelSandboxID],
	}
	if v, ok := labels[labelStartedAt]; ok {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			st.startedAt = ts
		}
	}
	return st
}

// labelInt64 formats an int64 as a string for use as a container label value.
func labelInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}

// readInt64Label reads an int64 value from a container label; returns 0 if absent or unparseable.
func readInt64Label(labels map[string]string, key string) int64 {
	if v, ok := labels[key]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return 0
}
