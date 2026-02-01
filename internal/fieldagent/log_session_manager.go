package fieldagent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/processmanager"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/eclipse-iofog/agent-go/pkg/docker"
)

const (
	logSessionManagerModuleName = "LogSessionManager"
)

// LogSession represents a log session from the controller
type LogSession struct {
	SessionID        string `json:"sessionId"`
	MicroserviceUUID string `json:"microserviceUuid,omitempty"`
	IofogUUID        string `json:"iofogUuid,omitempty"`
	Status           string `json:"status"` // ACTIVE, PENDING, etc.
}

// LogSessionInfo tracks information about an active log session
type LogSessionInfo struct {
	Session     *LogSession
	ContainerID string
	IsStreaming bool
}

// LogSessionManager manages active log sessions
type LogSessionManager struct {
	activeSessions      map[string]*LogSessionInfo
	webSocketHandlers   map[string]*LogSessionWebSocketHandler
	localLogReaders     map[string]*utils.LocalLogReader
	dockerTailCallbacks map[string]*dockerLogTailHandler
	processManager      *processmanager.ProcessManager
	dockerClient        *docker.Client
	mu                  sync.RWMutex
	fieldAgent          *FieldAgent
}

var (
	logSessionManagerInstance *LogSessionManager
	logSessionManagerOnce     sync.Once
)

// GetLogSessionManager returns the singleton LogSessionManager instance
func GetLogSessionManager() *LogSessionManager {
	logSessionManagerOnce.Do(func() {
		logSessionManagerInstance = &LogSessionManager{
			activeSessions:      make(map[string]*LogSessionInfo),
			webSocketHandlers:   make(map[string]*LogSessionWebSocketHandler),
			localLogReaders:     make(map[string]*utils.LocalLogReader),
			dockerTailCallbacks: make(map[string]*dockerLogTailHandler),
			dockerClient:        docker.GetInstance(),
			fieldAgent:          GetInstance(),
		}
	})
	return logSessionManagerInstance
}

// SetProcessManager sets the ProcessManager instance (called by supervisor)
func (lsm *LogSessionManager) SetProcessManager(pm *processmanager.ProcessManager) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	lsm.processManager = pm
}

// FetchLogSessions fetches log sessions from the controller
func (lsm *LogSessionManager) FetchLogSessions(ctx context.Context) ([]*LogSession, error) {
	logging.LogDebug(logSessionManagerModuleName, "Start fetching log sessions from controller")

	if lsm.fieldAgent.NotProvisioned() || !lsm.fieldAgent.IsControllerConnected(false) {
		logging.LogDebug(logSessionManagerModuleName, "Not provisioned or not connected, returning empty list")
		return []*LogSession{}, nil
	}

	// Make request to controller
	result, err := lsm.fieldAgent.apiClient.Request(ctx, "logs/sessions", GET, nil, nil)
	if err != nil {
		logging.LogError(logSessionManagerModuleName, "Unable to get log sessions", err)
		return nil, fmt.Errorf("unable to get log sessions: %w", err)
	}

	// Parse log sessions
	sessions := make([]*LogSession, 0)
	if logSessionsArray, ok := result["logSessions"].([]interface{}); ok {
		for _, sessionData := range logSessionsArray {
			if sessionMap, ok := sessionData.(map[string]interface{}); ok {
				session := lsm.parseLogSession(sessionMap)
				if session != nil {
					sessions = append(sessions, session)
				}
			}
		}
	}

	logging.LogDebug(logSessionManagerModuleName, fmt.Sprintf("Fetched %d log sessions from controller", len(sessions)))
	return sessions, nil
}

// parseLogSession parses a log session from JSON data
func (lsm *LogSessionManager) parseLogSession(data map[string]interface{}) *LogSession {
	session := &LogSession{}

	if sessionID, ok := data["sessionId"].(string); ok {
		session.SessionID = sessionID
	} else {
		logging.LogWarn(logSessionManagerModuleName, "Log session missing sessionId")
		return nil
	}

	if microserviceUUID, ok := data["microserviceUuid"].(string); ok && microserviceUUID != "" {
		session.MicroserviceUUID = microserviceUUID
	}

	if iofogUUID, ok := data["iofogUuid"].(string); ok && iofogUUID != "" {
		session.IofogUUID = iofogUUID
	}

	if status, ok := data["status"].(string); ok {
		session.Status = status
	} else {
		session.Status = "PENDING"
	}

	return session
}

// HandleLogSessions handles fetched log sessions from controller
func (lsm *LogSessionManager) HandleLogSessions(fetchedSessions []*LogSession) {
	logging.LogDebug(logSessionManagerModuleName, fmt.Sprintf("Handling log sessions: fetched=%d, active=%d", len(fetchedSessions), len(lsm.activeSessions)))

	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	// Create set of fetched session IDs
	fetchedSessionIDs := make(map[string]bool)
	for _, session := range fetchedSessions {
		fetchedSessionIDs[session.SessionID] = true
	}

	// Stop sessions that are no longer in fetched list
	activeSessionIDs := make([]string, 0, len(lsm.activeSessions))
	for sessionID := range lsm.activeSessions {
		activeSessionIDs = append(activeSessionIDs, sessionID)
	}

	for _, sessionID := range activeSessionIDs {
		if !fetchedSessionIDs[sessionID] {
			logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Stopping session no longer in controller response: %s", sessionID))
			lsm.stopLogSessionLocked(sessionID)
		}
	}

	// Start new sessions or update existing ones
	for _, session := range fetchedSessions {
		sessionID := session.SessionID
		if info, exists := lsm.activeSessions[sessionID]; exists {
			// Update existing session if needed
			info.Session = session
			if !info.IsStreaming && session.Status == "ACTIVE" {
				// Session was pending, now active - start streaming
				logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Session became active, starting stream: %s", sessionID))
				lsm.startLogStreamingLocked(sessionID)
			}
		} else {
			// New session - start it
			logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting new log session: %s", sessionID))
			lsm.startLogSessionLocked(session)
		}
	}
}

// startLogSessionLocked starts a log session (must be called with lock held)
func (lsm *LogSessionManager) startLogSessionLocked(session *LogSession) {
	sessionID := session.SessionID
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting log session: sessionId=%s, microserviceUuid=%s, iofogUuid=%s",
		sessionID, session.MicroserviceUUID, session.IofogUUID))

	// Create session info
	info := &LogSessionInfo{
		Session:     session,
		IsStreaming: false,
	}
	lsm.activeSessions[sessionID] = info

	// Determine if this is a microservice log or fog log
	isMicroserviceLog := session.MicroserviceUUID != ""

	// Create and connect WebSocket handler
	wsHandler := GetLogSessionWebSocketHandler(sessionID, session.MicroserviceUUID, session.IofogUUID, isMicroserviceLog)
	lsm.webSocketHandlers[sessionID] = wsHandler
	// Set LogSessionManager reference in handler so it can start tailing when ready (matching Java line 124)
	wsHandler.SetLogSessionManager(lsm)

	// Connect WebSocket
	err := wsHandler.Connect()
	if err != nil {
		logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Error connecting WebSocket for session: %s", sessionID), err)
		lsm.stopLogSessionLocked(sessionID)
		return
	}

	// DO NOT start streaming immediately - wait for LOG_START message from controller
	// Streaming will be started when WebSocket becomes active
	if !isMicroserviceLog && session.IofogUUID == "" {
		logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Invalid log session: neither microserviceUuid nor iofogUuid set: %s", sessionID), nil)
		lsm.stopLogSessionLocked(sessionID)
	}
}

// startLogStreamingLocked starts streaming logs for a session (must be called with lock held)
func (lsm *LogSessionManager) startLogStreamingLocked(sessionID string) {
	info, exists := lsm.activeSessions[sessionID]
	if !exists {
		logging.LogWarn(logSessionManagerModuleName, fmt.Sprintf("Cannot start streaming: session not found: %s", sessionID))
		return
	}

	if info.IsStreaming {
		logging.LogDebug(logSessionManagerModuleName, fmt.Sprintf("Already streaming: %s", sessionID))
		return
	}

	session := info.Session
	isMicroserviceLog := session.MicroserviceUUID != ""

	if isMicroserviceLog {
		lsm.startMicroserviceLogStreamingLocked(sessionID, session.MicroserviceUUID)
	} else {
		lsm.startFogLogStreamingLocked(sessionID, session.IofogUUID)
	}

	info.IsStreaming = true
}

// StartLogStreamingOnActivation starts log streaming when WebSocket becomes active
// This method is called from LogSessionWebSocketHandler when it receives LOG_START
// Matching Java: startLogStreamingOnActivation()
func (lsm *LogSessionManager) StartLogStreamingOnActivation(sessionID string, tailConfig map[string]interface{}) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()

	info, exists := lsm.activeSessions[sessionID]
	if !exists {
		logging.LogWarn(logSessionManagerModuleName, fmt.Sprintf("Session info not found for activation: %s", sessionID))
		return
	}

	if info.IsStreaming {
		logging.LogDebug(logSessionManagerModuleName, fmt.Sprintf("Session already streaming: %s", sessionID))
		return
	}

	session := info.Session
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting log streaming on WebSocket activation: sessionId=%s", sessionID))

	// Update session with tailConfig from LOG_START if provided (matching Java line 445-448)
	// Note: LogSession struct doesn't have TailConfig field yet, but we can store it in session info if needed

	if session.MicroserviceUUID != "" {
		lsm.startMicroserviceLogStreamingLocked(sessionID, session.MicroserviceUUID)
	} else if session.IofogUUID != "" {
		lsm.startFogLogStreamingLocked(sessionID, session.IofogUUID)
	}

	info.IsStreaming = true
}

// startMicroserviceLogStreamingLocked starts streaming microservice logs (must be called with lock held)
func (lsm *LogSessionManager) startMicroserviceLogStreamingLocked(sessionID, microserviceUUID string) {
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting microservice log streaming: sessionId=%s, microserviceUuid=%s", sessionID, microserviceUUID))

	// Get container ID from ProcessManager
	if lsm.processManager == nil {
		logging.LogError(logSessionManagerModuleName, "ProcessManager not set, cannot start microservice log streaming", fmt.Errorf("sessionID: %s", sessionID))
		return
	}

	// Get container by microservice UUID
	container, err := lsm.dockerClient.GetContainer(microserviceUUID)
	if err != nil {
		logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Error getting container for microservice: %s", microserviceUUID), err)
		return
	}

	if container == nil {
		logging.LogWarn(logSessionManagerModuleName, fmt.Sprintf("Container not found for microservice: %s", microserviceUUID))
		return
	}

	containerID := container.ID
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting Docker log tailing: sessionId=%s, containerId=%s", sessionID, containerID))

	// Get WebSocket handler
	wsHandler := lsm.webSocketHandlers[sessionID]
	if wsHandler == nil {
		logging.LogError(logSessionManagerModuleName, "WebSocket handler not found for session", fmt.Errorf("sessionID: %s", sessionID))
		return
	}

	// Get tail config from WebSocket handler (if available)
	tailConfig := &docker.TailConfig{
		Follow: true,
		Lines:  100,
	}

	// Create log tail handler
	handler := &dockerLogTailHandler{
		sessionID:        sessionID,
		microserviceUUID: microserviceUUID,
		wsHandler:        wsHandler,
		lsm:              lsm,
	}

	// Store handler
	lsm.dockerTailCallbacks[sessionID] = handler

	// Start tailing
	err = lsm.dockerClient.TailContainerLogs(containerID, sessionID, microserviceUUID, handler, tailConfig)
	if err != nil {
		logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Error starting Docker log tailing: sessionId=%s", sessionID), err)
		if info, ok := lsm.activeSessions[sessionID]; ok {
			info.IsStreaming = false
		}
		delete(lsm.dockerTailCallbacks, sessionID)
		return
	}

	// Update session info
	if info, ok := lsm.activeSessions[sessionID]; ok {
		info.ContainerID = containerID
		info.IsStreaming = true
	}

	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Docker log tailing started: sessionId=%s", sessionID))
}

// dockerLogTailHandler implements docker.LogTailHandler interface
type dockerLogTailHandler struct {
	sessionID        string
	microserviceUUID string
	wsHandler        *LogSessionWebSocketHandler
	lsm              *LogSessionManager
}

func (h *dockerLogTailHandler) OnLogLine(sessionID, microserviceUUID string, lineBytes []byte, streamType docker.StreamType) {
	if h.wsHandler != nil && h.wsHandler.IsActive() {
		// Send log line via WebSocket
		msgType := byte(6) // LogTypeLogLine
		if err := h.wsHandler.SendMessage(msgType, lineBytes); err != nil {
			logging.LogError(logSessionManagerModuleName, "Error sending log line", err)
		}
	}
}

func (h *dockerLogTailHandler) OnComplete(sessionID string) {
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Docker log tailing completed: sessionId=%s", sessionID))
	h.lsm.mu.Lock()
	defer h.lsm.mu.Unlock()

	if info, ok := h.lsm.activeSessions[sessionID]; ok {
		info.IsStreaming = false
	}

	// Remove handler
	delete(h.lsm.dockerTailCallbacks, sessionID)
}

func (h *dockerLogTailHandler) OnError(sessionID string, err error) {
	logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Docker log tailing error: sessionId=%s", sessionID), err)
	h.lsm.mu.Lock()
	defer h.lsm.mu.Unlock()

	if info, ok := h.lsm.activeSessions[sessionID]; ok {
		info.IsStreaming = false
	}

	// Remove handler
	delete(h.lsm.dockerTailCallbacks, sessionID)
}

// startFogLogStreamingLocked starts streaming fog logs (must be called with lock held)
func (lsm *LogSessionManager) startFogLogStreamingLocked(sessionID, iofogUUID string) {
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting fog log streaming: sessionId=%s, iofogUuid=%s", sessionID, iofogUUID))

	// Get WebSocket handler
	wsHandler := lsm.webSocketHandlers[sessionID]
	if wsHandler == nil {
		logging.LogError(logSessionManagerModuleName, "WebSocket handler not found for session", fmt.Errorf("sessionID: %s", sessionID))
		return
	}

	// Create LocalLogReader handler
	handler := &fogLogHandler{
		sessionID: sessionID,
		iofogUUID: iofogUUID,
		wsHandler: wsHandler,
		lsm:       lsm,
	}

	// Create tail config (default: follow=true, lines=100)
	tailConfig := &utils.TailConfig{
		Follow: true,
		Lines:  100,
	}

	// Get log directory from config
	cfg := config.GetInstance()
	logDir := cfg.LogDiskDirectory

	// Create and start LocalLogReader
	reader := utils.NewLocalLogReader(sessionID, iofogUUID, logDir, tailConfig, handler)
	lsm.localLogReaders[sessionID] = reader
	reader.Start()

	logging.LogDebug(logSessionManagerModuleName, fmt.Sprintf("Started LocalLogReader for fog logs: sessionId=%s", sessionID))
}

// fogLogHandler implements LocalLogHandler interface for fog logs
type fogLogHandler struct {
	sessionID string
	iofogUUID string
	wsHandler *LogSessionWebSocketHandler
	lsm       *LogSessionManager
}

func (h *fogLogHandler) OnLogLine(sessionID, iofogUUID, line string) {
	if h.wsHandler != nil && h.wsHandler.IsActive() {
		// Send log line via WebSocket
		lineBytes := []byte(line)
		msgType := byte(6) // LogTypeLogLine
		if err := h.wsHandler.SendMessage(msgType, lineBytes); err != nil {
			logging.LogError(logSessionManagerModuleName, "Error sending log line", err)
		}
	}
}

func (h *fogLogHandler) OnComplete(sessionID string) {
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Local log reading completed: sessionId=%s", sessionID))
	h.lsm.mu.Lock()
	defer h.lsm.mu.Unlock()

	if info, ok := h.lsm.activeSessions[sessionID]; ok {
		info.IsStreaming = false
	}

	// Remove reader
	delete(h.lsm.localLogReaders, sessionID)
}

func (h *fogLogHandler) OnError(sessionID string, err error) {
	logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Local log reading error: sessionId=%s", sessionID), err)
	h.lsm.mu.Lock()
	defer h.lsm.mu.Unlock()

	if info, ok := h.lsm.activeSessions[sessionID]; ok {
		info.IsStreaming = false
	}

	// Remove reader
	delete(h.lsm.localLogReaders, sessionID)
}

// stopLogSessionLocked stops a log session (must be called with lock held)
func (lsm *LogSessionManager) stopLogSessionLocked(sessionID string) {
	// Stop LocalLogReader if it exists
	if reader, ok := lsm.localLogReaders[sessionID]; ok {
		reader.Stop()
		delete(lsm.localLogReaders, sessionID)
	}

	// Remove Docker tail callback if it exists
	delete(lsm.dockerTailCallbacks, sessionID)

	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Stopping log session: %s", sessionID))

	// Stop WebSocket handler
	if handler, exists := lsm.webSocketHandlers[sessionID]; exists {
		handler.Disconnect()
		delete(lsm.webSocketHandlers, sessionID)
	}

	// Remove from active sessions
	delete(lsm.activeSessions, sessionID)

	logging.LogDebug(logSessionManagerModuleName, fmt.Sprintf("Stopped log session: %s", sessionID))
}

// StopLogSession stops a log session (thread-safe)
func (lsm *LogSessionManager) StopLogSession(sessionID string) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	lsm.stopLogSessionLocked(sessionID)
}

// StartWorker starts the background worker that periodically fetches and handles log sessions
func (lsm *LogSessionManager) StartWorker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Fetch every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessions, err := lsm.FetchLogSessions(ctx)
			if err != nil {
				logging.LogError(logSessionManagerModuleName, "Error fetching log sessions", err)
				continue
			}
			lsm.HandleLogSessions(sessions)
		}
	}
}
