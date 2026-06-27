package processmanager

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

const ExecStartGateTimeout = 15 // seconds — use with time.Second in callers

// ExecOwner identifies who owns an interactive exec session.
type ExecOwner string

const (
	ExecOwnerController ExecOwner = "controller"
	ExecOwnerLocal      ExecOwner = "local"
)

// ExecSessionRecord tracks one interactive exec session in the ProcessManager registry.
type ExecSessionRecord struct {
	SessionID     string
	MSUUID        string
	ContainerID   string
	RuntimeExecID string
	Owner         ExecOwner
	Started       bool
}

var (
	ErrExecSessionNotFound      = errors.New("exec session not found")
	ErrExecSessionOwnerMismatch = errors.New("exec session owner mismatch")
	ErrExecSessionDuplicate     = errors.New("exec session already registered")
	ErrExecStartTimeout         = errors.New("exec start timeout")
)

// ExecSessionRegistry tracks interactive exec sessions keyed by wire sessionId.
type ExecSessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*ExecSessionRecord
}

// NewExecSessionRegistry returns an empty exec session registry.
func NewExecSessionRegistry() *ExecSessionRegistry {
	return &ExecSessionRegistry{
		sessions: make(map[string]*ExecSessionRecord),
	}
}

// Register inserts a pending session record. sessionId must be unique.
func (r *ExecSessionRegistry) Register(rec *ExecSessionRecord) error {
	if r == nil || rec == nil {
		return errors.New("exec session record is nil")
	}
	sessionID := strings.TrimSpace(rec.SessionID)
	if sessionID == "" {
		return errors.New("exec session id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[sessionID]; exists {
		return ErrExecSessionDuplicate
	}
	stored := *rec
	stored.SessionID = sessionID
	r.sessions[sessionID] = &stored
	return nil
}

// SetRuntimeExecID updates the engine-native exec handle after CreateExecSession.
// Edgelet engines use the pre-create hint; docker/podman replace it with the daemon-assigned id.
func (r *ExecSessionRegistry) SetRuntimeExecID(sessionID, engineExecID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	engineExecID = strings.TrimSpace(engineExecID)
	if sessionID == "" || engineExecID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.sessions[sessionID]; ok && rec != nil {
		rec.RuntimeExecID = engineExecID
	}
}

// MarkStarted marks a session as successfully started.
func (r *ExecSessionRegistry) MarkStarted(sessionID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.sessions[sessionID]; ok {
		rec.Started = true
	}
}

// Release removes a session when owner matches.
func (r *ExecSessionRegistry) Release(sessionID string, owner ExecOwner) (*ExecSessionRecord, error) {
	if r == nil {
		return nil, ErrExecSessionNotFound
	}
	sessionID = strings.TrimSpace(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.sessions[sessionID]
	if !ok {
		return nil, ErrExecSessionNotFound
	}
	if rec.Owner != owner {
		return nil, ErrExecSessionOwnerMismatch
	}
	delete(r.sessions, sessionID)
	copied := *rec
	return &copied, nil
}

// Remove deletes a session regardless of owner (internal cleanup).
func (r *ExecSessionRegistry) Remove(sessionID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, sessionID)
}

// Get returns a copy of the session record.
func (r *ExecSessionRegistry) Get(sessionID string) (*ExecSessionRecord, bool) {
	if r == nil {
		return nil, false
	}
	sessionID = strings.TrimSpace(sessionID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.sessions[sessionID]
	if !ok {
		return nil, false
	}
	copied := *rec
	return &copied, true
}

// ListControllerSessionsForMS returns controller-owned sessions for one microservice.
func (r *ExecSessionRegistry) ListControllerSessionsForMS(msUUID string) []ExecSessionRecord {
	return r.listForMS(msUUID, ExecOwnerController)
}

// ListInteractiveForMS returns all controller and local interactive sessions for one microservice.
func (r *ExecSessionRegistry) ListInteractiveForMS(msUUID string) []ExecSessionRecord {
	if r == nil {
		return nil
	}
	msUUID = strings.TrimSpace(msUUID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ExecSessionRecord, 0)
	for _, rec := range r.sessions {
		if rec == nil {
			continue
		}
		if strings.TrimSpace(rec.MSUUID) != msUUID {
			continue
		}
		if rec.Owner != ExecOwnerController && rec.Owner != ExecOwnerLocal {
			continue
		}
		out = append(out, *rec)
	}
	return out
}

func (r *ExecSessionRegistry) listForMS(msUUID string, owner ExecOwner) []ExecSessionRecord {
	if r == nil {
		return nil
	}
	msUUID = strings.TrimSpace(msUUID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ExecSessionRecord, 0)
	for _, rec := range r.sessions {
		if rec == nil {
			continue
		}
		if strings.TrimSpace(rec.MSUUID) != msUUID {
			continue
		}
		if rec.Owner != owner {
			continue
		}
		out = append(out, *rec)
	}
	return out
}

// RuntimeExecIDsForContainer returns registered runtime exec ids for a container (for orphan sweep).
func (r *ExecSessionRegistry) RuntimeExecIDsForContainer(containerID string) map[string]struct{} {
	if r == nil {
		return map[string]struct{}{}
	}
	containerID = strings.TrimSpace(containerID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]struct{})
	for _, rec := range r.sessions {
		if rec == nil {
			continue
		}
		if strings.TrimSpace(rec.ContainerID) != containerID {
			continue
		}
		runtimeID := strings.TrimSpace(rec.RuntimeExecID)
		if runtimeID == "" {
			continue
		}
		out[runtimeID] = struct{}{}
	}
	return out
}

func runtimeExecIDController(containerID, sessionID string) string {
	return fmt.Sprintf("%s-exec-%s", containerIDPrefix(containerID), sessionPrefix(sessionID))
}

func runtimeExecIDLocal(containerID, localSessionID string) string {
	return fmt.Sprintf("%s-local-%s", containerIDPrefix(containerID), sessionPrefix(localSessionID))
}

func containerIDPrefix(containerID string) string {
	containerID = strings.TrimSpace(containerID)
	if len(containerID) > 12 {
		return containerID[:12]
	}
	return containerID
}

func sessionPrefix(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if len(sessionID) > 8 {
		return sessionID[:8]
	}
	return sessionID
}
