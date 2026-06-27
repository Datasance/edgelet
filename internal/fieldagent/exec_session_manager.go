package fieldagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

const (
	execSessionManagerModuleName   = "ExecSessionManager"
	maxControllerExecSessionsPerMS = 3
)

// ExecSession represents a controller exec session row from poll.
type ExecSession struct {
	SessionID        string `json:"sessionId"`
	MicroserviceUUID string `json:"microserviceUuid"`
	Status           string `json:"status"` // PENDING, ACTIVE
	AgentConnected   bool   `json:"agentConnected"`
}

// ExecSessionInfo tracks a local controller exec attachment.
type ExecSessionInfo struct {
	Session  *ExecSession
	Callback *ExecSessionCallback
}

// ExecSessionManager manages controller exec session attachments (log-session parity).
type ExecSessionManager struct {
	activeSessions    map[string]*ExecSessionInfo
	webSocketHandlers map[string]*ExecSessionWebSocketHandler
	processManager    *processmanager.ProcessManager
	mu                sync.RWMutex
	fieldAgent        *FieldAgent
}

var (
	execSessionManagerInstance *ExecSessionManager
	execSessionManagerOnce     sync.Once
)

// GetExecSessionManager returns the singleton ExecSessionManager instance.
func GetExecSessionManager() *ExecSessionManager {
	execSessionManagerOnce.Do(func() {
		execSessionManagerInstance = &ExecSessionManager{
			activeSessions:    make(map[string]*ExecSessionInfo),
			webSocketHandlers: make(map[string]*ExecSessionWebSocketHandler),
			fieldAgent:        GetInstance(),
		}
	})
	return execSessionManagerInstance
}

// SetProcessManager sets the ProcessManager instance (called by supervisor).
func (esm *ExecSessionManager) SetProcessManager(pm *processmanager.ProcessManager) {
	esm.mu.Lock()
	defer esm.mu.Unlock()
	esm.processManager = pm
}

// FetchExecSessions fetches exec sessions from the controller.
func (esm *ExecSessionManager) FetchExecSessions(ctx context.Context) ([]*ExecSession, error) {
	logging.LogDebug(execSessionManagerModuleName, "Start fetching exec sessions from controller")

	if esm.fieldAgent.NotProvisioned() || !esm.fieldAgent.IsControllerConnected(false) {
		logging.LogDebug(execSessionManagerModuleName, "Not provisioned or not connected, returning empty list")
		return []*ExecSession{}, nil
	}

	result, err := esm.fieldAgent.apiClient.Request(ctx, "exec/sessions", GET, nil, nil)
	if err != nil {
		logging.LogError(execSessionManagerModuleName, "Unable to get exec sessions", err)
		return nil, fmt.Errorf("unable to get exec sessions: %w", err)
	}

	sessions := make([]*ExecSession, 0)
	if execSessionsArray, ok := result["execSessions"].([]any); ok {
		for _, sessionData := range execSessionsArray {
			if sessionMap, ok := sessionData.(map[string]any); ok {
				session := esm.parseExecSession(sessionMap)
				if session != nil {
					sessions = append(sessions, session)
				}
			}
		}
	}

	logging.LogDebug(execSessionManagerModuleName, fmt.Sprintf("Fetched %d exec sessions from controller", len(sessions)))
	return sessions, nil
}

func (esm *ExecSessionManager) parseExecSession(data map[string]any) *ExecSession {
	session := &ExecSession{}

	if sessionID, ok := data["sessionId"].(string); ok {
		session.SessionID = sessionID
	} else {
		logging.LogWarn(execSessionManagerModuleName, "Exec session missing sessionId")
		return nil
	}

	if microserviceUUID, ok := data["microserviceUuid"].(string); ok {
		session.MicroserviceUUID = microserviceUUID
	}
	if session.MicroserviceUUID == "" {
		logging.LogWarn(execSessionManagerModuleName, fmt.Sprintf("Exec session missing microserviceUuid: %s", session.SessionID))
		return nil
	}

	if status, ok := data["status"].(string); ok {
		session.Status = status
	} else {
		session.Status = "PENDING"
	}

	if agentConnected, ok := data["agentConnected"].(bool); ok {
		session.AgentConnected = agentConnected
	}

	return session
}

// HandleExecSessions reconciles local controller exec attachments with polled rows.
func (esm *ExecSessionManager) HandleExecSessions(fetchedSessions []*ExecSession) {
	esm.mu.Lock()

	activeCount := len(esm.activeSessions)
	esm.mu.Unlock()

	logging.LogDebug(execSessionManagerModuleName, fmt.Sprintf("Handling exec sessions: fetched=%d, active=%d", len(fetchedSessions), activeCount))

	esm.mu.Lock()

	fetchedSessionIDs := make(map[string]bool, len(fetchedSessions))
	for _, session := range fetchedSessions {
		fetchedSessionIDs[session.SessionID] = true
	}

	activeSessionIDs := make([]string, 0, len(esm.activeSessions))
	for sessionID := range esm.activeSessions {
		activeSessionIDs = append(activeSessionIDs, sessionID)
	}

	esm.mu.Unlock()

	for _, sessionID := range activeSessionIDs {
		if !fetchedSessionIDs[sessionID] {
			logging.LogInfo(execSessionManagerModuleName, fmt.Sprintf("Stopping exec session no longer in controller response: %s", sessionID))
			esm.stopExecSession(sessionID)
		}
	}

	esm.mu.Lock()
	defer esm.mu.Unlock()

	for _, session := range fetchedSessions {
		sessionID := session.SessionID
		if info, exists := esm.activeSessions[sessionID]; exists {
			info.Session = session
			wsHandler := esm.webSocketHandlers[sessionID]
			if wsHandler != nil && !wsHandler.IsConnected() {
				logging.LogInfo(execSessionManagerModuleName, fmt.Sprintf("Reconnecting exec WebSocket for session: %s", sessionID))
				esm.reconnectExecSessionLocked(sessionID)
			}
			continue
		}

		logging.LogInfo(execSessionManagerModuleName, fmt.Sprintf("Starting new exec session: %s", sessionID))
		esm.startExecSessionLocked(session)
	}
}

func defaultExecShellCommand() []string {
	return []string{"sh", "-c", "clear; (bash || ash || sh)"}
}

func (esm *ExecSessionManager) countControllerSessionsForMS(msUUID string) int {
	count := 0
	for _, info := range esm.activeSessions {
		if info.Session != nil && info.Session.MicroserviceUUID == msUUID {
			count++
		}
	}
	return count
}

// startExecSessionLocked starts a controller exec attachment (must be called with lock held).
func (esm *ExecSessionManager) startExecSessionLocked(session *ExecSession) {
	sessionID := session.SessionID
	msUUID := session.MicroserviceUUID

	if esm.countControllerSessionsForMS(msUUID) >= maxControllerExecSessionsPerMS {
		logging.LogWarn(execSessionManagerModuleName, fmt.Sprintf("Refusing exec attach: microservice %s already has %d controller sessions", msUUID, maxControllerExecSessionsPerMS))
		return
	}

	if esm.processManager == nil {
		logging.LogError(execSessionManagerModuleName, "ProcessManager not set, cannot start exec session", fmt.Errorf("sessionID: %s", sessionID))
		return
	}

	callback := NewExecSessionCallback(msUUID, sessionID)
	esm.activeSessions[sessionID] = &ExecSessionInfo{
		Session:  session,
		Callback: callback,
	}

	wsHandler := GetExecSessionWebSocketHandler(sessionID, msUUID)
	if wsHandler == nil {
		logging.LogError(execSessionManagerModuleName, "WebSocket handler not created (controller URL empty)", fmt.Errorf("sessionID: %s", sessionID))
		esm.stopExecSessionLocked(sessionID)
		return
	}
	wsHandler.SetExecSessionManager(esm)
	esm.webSocketHandlers[sessionID] = wsHandler

	command := defaultExecShellCommand()
	pm := esm.processManager
	if wsHandler.needsResetBeforeConnect() {
		wsHandler.Reset()
	}
	esm.mu.Unlock()
	if err := wsHandler.Connect(); err != nil {
		esm.mu.Lock()
		logging.LogError(execSessionManagerModuleName, fmt.Sprintf("Error connecting WebSocket for exec session: %s", sessionID), err)
		esm.stopExecSessionLocked(sessionID)
		return
	}
	err := pm.CreateControllerExecSession(sessionID, msUUID, command, callback)
	esm.mu.Lock()

	if err != nil {
		logging.LogError(execSessionManagerModuleName, fmt.Sprintf("Failed to create controller exec session: %s", sessionID), err)
		esm.stopExecSessionLocked(sessionID)
		return
	}

	logging.LogInfo(execSessionManagerModuleName, fmt.Sprintf("Exec session started: sessionId=%s, microserviceUuid=%s", sessionID, msUUID))
}

func (esm *ExecSessionManager) reconnectExecSessionLocked(sessionID string) {
	wsHandler := esm.webSocketHandlers[sessionID]
	if wsHandler == nil {
		return
	}
	if wsHandler.IsConnected() {
		return
	}
	esm.mu.Unlock()
	wsHandler.Reset()
	err := wsHandler.Connect()
	esm.mu.Lock()
	if err != nil {
		logging.LogError(execSessionManagerModuleName, fmt.Sprintf("Failed to reconnect exec WebSocket: %s", sessionID), err)
	}
}

func (esm *ExecSessionManager) removeExecSessionStateLocked(sessionID string) (*ExecSessionCallback, *ExecSessionWebSocketHandler, *processmanager.ProcessManager) {
	var callback *ExecSessionCallback
	if info, ok := esm.activeSessions[sessionID]; ok {
		callback = info.Callback
	}
	handler := esm.webSocketHandlers[sessionID]
	delete(esm.webSocketHandlers, sessionID)
	delete(esm.activeSessions, sessionID)
	return callback, handler, esm.processManager
}

func (esm *ExecSessionManager) teardownExecSession(sessionID string, callback *ExecSessionCallback, handler *ExecSessionWebSocketHandler, pm *processmanager.ProcessManager) {
	logging.LogInfo(execSessionManagerModuleName, fmt.Sprintf("Stopping exec session: %s", sessionID))
	if callback != nil {
		callback.Close()
	}
	if handler != nil {
		handler.Disconnect()
	}
	if pm != nil {
		if err := pm.ReleaseExecSession(sessionID, processmanager.ExecOwnerController); err != nil {
			logging.LogWarn(execSessionManagerModuleName, fmt.Sprintf("ReleaseExecSession session=%s: %v", sessionID, err))
		}
	}
}

// stopExecSessionLocked removes session state under esm.mu and tears down I/O without holding the lock.
// Must be called with esm.mu held; re-acquires esm.mu before return.
func (esm *ExecSessionManager) stopExecSessionLocked(sessionID string) {
	callback, handler, pm := esm.removeExecSessionStateLocked(sessionID)
	esm.mu.Unlock()
	esm.teardownExecSession(sessionID, callback, handler, pm)
	esm.mu.Lock()
}

// stopExecSession stops one controller exec attachment without requiring esm.mu to be held.
func (esm *ExecSessionManager) stopExecSession(sessionID string) {
	esm.mu.Lock()
	callback, handler, pm := esm.removeExecSessionStateLocked(sessionID)
	esm.mu.Unlock()
	esm.teardownExecSession(sessionID, callback, handler, pm)
}

// StopExecSession stops one controller exec attachment (thread-safe).
func (esm *ExecSessionManager) StopExecSession(sessionID string) {
	esm.stopExecSession(sessionID)
}

// GetCallback returns the exec callback for a session id.
func (esm *ExecSessionManager) GetCallback(sessionID string) *ExecSessionCallback {
	esm.mu.RLock()
	defer esm.mu.RUnlock()
	if info, ok := esm.activeSessions[sessionID]; ok {
		return info.Callback
	}
	return nil
}

// StopAllInteractiveForMicroservice stops all exec attachments and runtime sessions for one MS.
func (esm *ExecSessionManager) StopAllInteractiveForMicroservice(msUUID string) {
	esm.mu.Lock()
	var sessionIDs []string
	for sessionID, info := range esm.activeSessions {
		if info.Session != nil && info.Session.MicroserviceUUID == msUUID {
			sessionIDs = append(sessionIDs, sessionID)
		}
	}
	for _, sessionID := range sessionIDs {
		esm.stopExecSessionLocked(sessionID)
	}
	pm := esm.processManager
	esm.mu.Unlock()

	if pm != nil {
		pm.StopAllInteractiveForMicroservice(msUUID)
	}
}
