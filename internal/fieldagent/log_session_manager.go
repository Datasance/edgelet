package fieldagent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/utils"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
)

// parseTailConfig extracts follow, lines, since, until from tailConfigMap (from LOG_START).
// Returns defaults (follow=true, lines=100) when map is nil or empty.
func parseTailConfig(tailConfigMap map[string]any) *engine.TailConfig {
	cfg := &engine.TailConfig{
		Follow: true,
		Lines:  100,
	}
	if tailConfigMap == nil {
		return cfg
	}
	if v, ok := tailConfigMap["follow"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Follow = b
		}
	}
	if v, ok := tailConfigMap["lines"]; ok {
		switch n := v.(type) {
		case float64:
			cfg.Lines = int(n)
		case int:
			cfg.Lines = n
		case int64:
			cfg.Lines = int(n)
		}
		if cfg.Lines < 1 {
			cfg.Lines = 100
		}
		if cfg.Lines > 10000 {
			cfg.Lines = 10000
		}
	}
	if v, ok := tailConfigMap["since"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Since = s
		}
	}
	if v, ok := tailConfigMap["until"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Until = s
		}
	}
	return cfg
}

// engineTailConfigToUtils converts engine.TailConfig to utils.TailConfig for LocalLogReader.
func engineTailConfigToUtils(cfg *engine.TailConfig) *utils.TailConfig {
	if cfg == nil {
		return &utils.TailConfig{Follow: true, Lines: 100}
	}
	return &utils.TailConfig{
		Follow: cfg.Follow,
		Lines:  cfg.Lines,
		Since:  cfg.Since,
		Until:  cfg.Until,
	}
}

const (
	logSessionManagerModuleName = "LogSessionManager"
	logSessionMaxDuration       = ControllerSessionMaxDuration
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
	Session            *LogSession
	ContainerID        string
	IsStreaming        bool
	streamingStartedAt time.Time
	tailCancel         context.CancelFunc
}

func (info *LogSessionInfo) markStreamingStarted() {
	info.IsStreaming = true
	info.streamingStartedAt = time.Now()
}

// LogSessionManager manages active log sessions
type LogSessionManager struct {
	activeSessions    map[string]*LogSessionInfo
	webSocketHandlers map[string]*LogSessionWebSocketHandler
	localLogReaders   map[string]*utils.LocalLogReader
	tailCallbacks     map[string]*logTailHandler
	processManager    *processmanager.ProcessManager
	containerEngine   engine.ContainerEngine
	mu                sync.RWMutex
	fieldAgent        *FieldAgent
}

var (
	logSessionManagerInstance *LogSessionManager
	logSessionManagerOnce     sync.Once
)

// GetLogSessionManager returns the singleton LogSessionManager instance.
func GetLogSessionManager() *LogSessionManager {
	logSessionManagerOnce.Do(func() {
		logSessionManagerInstance = &LogSessionManager{
			activeSessions:    make(map[string]*LogSessionInfo),
			webSocketHandlers: make(map[string]*LogSessionWebSocketHandler),
			localLogReaders:   make(map[string]*utils.LocalLogReader),
			tailCallbacks:     make(map[string]*logTailHandler),
			fieldAgent:        GetInstance(),
		}
	})
	return logSessionManagerInstance
}

// SetProcessManager sets the ProcessManager instance (called by supervisor).
func (lsm *LogSessionManager) SetProcessManager(pm *processmanager.ProcessManager) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	lsm.processManager = pm
}

// SetEngine sets the ContainerEngine used to look up containers and tail their logs.
// Must be called before log session streaming starts (from supervisor after engine init).
func (lsm *LogSessionManager) SetEngine(e engine.ContainerEngine) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	lsm.containerEngine = e
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
	if logSessionsArray, ok := result["logSessions"].([]any); ok {
		for _, sessionData := range logSessionsArray {
			if sessionMap, ok := sessionData.(map[string]any); ok {
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
func (lsm *LogSessionManager) parseLogSession(data map[string]any) *LogSession {
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
			wsHandler := lsm.webSocketHandlers[sessionID]
			if wsHandler != nil && !wsHandler.IsConnected() {
				logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Reconnecting log WebSocket for session: %s", sessionID))
				lsm.reconnectLogSessionLocked(sessionID)
			}
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
	if wsHandler == nil {
		logging.LogError(logSessionManagerModuleName, "WebSocket handler not created (controller URL empty)", fmt.Errorf("sessionID: %s", sessionID))
		lsm.stopLogSessionLocked(sessionID)
		return
	}
	lsm.webSocketHandlers[sessionID] = wsHandler
	// Set LogSessionManager reference in handler so it can start tailing when ready
	wsHandler.SetLogSessionManager(lsm)

	wsHandler.Reset()
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

	// startLogStreamingLocked is called when session becomes ACTIVE (before LOG_START);
	// use default tailConfig. LOG_START path uses StartLogStreamingOnActivation with parsed config.
	defaultTailConfig := parseTailConfig(nil)

	if isMicroserviceLog {
		lsm.startMicroserviceLogStreamingLocked(sessionID, session.MicroserviceUUID, defaultTailConfig)
	} else {
		lsm.startFogLogStreamingLocked(sessionID, session.IofogUUID, engineTailConfigToUtils(defaultTailConfig))
	}

	info.markStreamingStarted()
}

// StartLogStreamingOnActivation starts log streaming when WebSocket becomes active
// This method is called from LogSessionWebSocketHandler when it receives LOG_START
func (lsm *LogSessionManager) StartLogStreamingOnActivation(sessionID string, tailConfigMap map[string]any) {
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

	tailConfig := parseTailConfig(tailConfigMap)

	if session.MicroserviceUUID != "" {
		lsm.startMicroserviceLogStreamingLocked(sessionID, session.MicroserviceUUID, tailConfig)
	} else if session.IofogUUID != "" {
		lsm.startFogLogStreamingLocked(sessionID, session.IofogUUID, engineTailConfigToUtils(tailConfig))
	}

	info.markStreamingStarted()
}

// startMicroserviceLogStreamingLocked starts streaming microservice logs (must be called with lock held)
func (lsm *LogSessionManager) startMicroserviceLogStreamingLocked(sessionID, microserviceUUID string, tailConfig *engine.TailConfig) {
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting microservice log streaming: sessionId=%s, microserviceUuid=%s", sessionID, microserviceUUID))

	if lsm.processManager == nil {
		logging.LogError(logSessionManagerModuleName, "ProcessManager not set, cannot start microservice log streaming", fmt.Errorf("sessionID: %s", sessionID))
		return
	}

	if lsm.containerEngine == nil {
		logging.LogError(logSessionManagerModuleName, "ContainerEngine not set, cannot start microservice log streaming", fmt.Errorf("sessionID: %s", sessionID))
		return
	}

	lsm.stopLogStreamingLocked(sessionID)

	// Look up container via the engine abstraction (works for Docker, iofog, and all others)
	container, err := lsm.containerEngine.GetContainer(microserviceUUID)
	if err != nil {
		logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Error getting container for microservice: %s", microserviceUUID), err)
		return
	}

	if container == nil {
		logging.LogWarn(logSessionManagerModuleName, fmt.Sprintf("Container not found for microservice: %s", microserviceUUID))
		return
	}

	containerID := container.ID
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting log tailing: sessionId=%s, containerId=%s", sessionID, containerID))

	wsHandler := lsm.webSocketHandlers[sessionID]
	if wsHandler == nil {
		logging.LogError(logSessionManagerModuleName, "WebSocket handler not found for session", fmt.Errorf("sessionID: %s", sessionID))
		return
	}

	if tailConfig == nil {
		tailConfig = parseTailConfig(nil)
	} else {
		cfgCopy := *tailConfig
		tailConfig = &cfgCopy
	}

	tailCtx, tailCancel := context.WithCancel(context.Background())
	handedOff := false
	defer func() {
		if !handedOff {
			tailCancel()
		}
	}()

	tailConfig.Ctx = tailCtx
	info, ok := lsm.activeSessions[sessionID]
	if !ok {
		return
	}
	info.tailCancel = tailCancel
	handedOff = true

	handler := &logTailHandler{
		sessionID:        sessionID,
		microserviceUUID: microserviceUUID,
		wsHandler:        wsHandler,
		lsm:              lsm,
	}

	lsm.tailCallbacks[sessionID] = handler

	info.ContainerID = containerID
	info.markStreamingStarted()

	// Run TailContainerLogs in a goroutine — it blocks in follow mode until the session ends.
	// If called synchronously, lsm.mu would be held for the entire stream, deadlocking
	// HandleLogSessions on the next getChangesList cycle.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.LogError(logSessionManagerModuleName, "Panic recovered", fmt.Errorf("%v", r))
			}
		}()
		err := lsm.containerEngine.TailContainerLogs(containerID, sessionID, microserviceUUID, handler, tailConfig)
		if err != nil {
			logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Error starting log tailing: sessionId=%s", sessionID), err)
			lsm.mu.Lock()
			if info, ok := lsm.activeSessions[sessionID]; ok {
				info.IsStreaming = false
			}
			delete(lsm.tailCallbacks, sessionID)
			lsm.mu.Unlock()
			return
		}
		// OnComplete is called by TailContainerLogs when done; handler updates state
	}()

	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Log tailing started: sessionId=%s", sessionID))
}

// logTailHandler implements engine.LogTailHandler and forwards log lines to the WebSocket.
type logTailHandler struct {
	sessionID        string
	microserviceUUID string
	wsHandler        *LogSessionWebSocketHandler
	lsm              *LogSessionManager
	stopped          atomic.Bool
}

func (h *logTailHandler) OnLogLine(_, _ string, lineBytes []byte, _ engine.StreamType) {
	if h.stopped.Load() || h.wsHandler == nil || len(lineBytes) == 0 {
		return
	}
	msgType := byte(6) // LogTypeLogLine
	_ = h.wsHandler.SendMessage(msgType, lineBytes)
}

func (h *logTailHandler) OnComplete(sessionID string) {
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Log tailing completed: sessionId=%s", sessionID))
	h.lsm.mu.Lock()
	defer h.lsm.mu.Unlock()

	if info, ok := h.lsm.activeSessions[sessionID]; ok {
		info.IsStreaming = false
	}
	delete(h.lsm.tailCallbacks, sessionID)
}

func (h *logTailHandler) OnError(sessionID string, err error) {
	logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Log tailing error: sessionId=%s", sessionID), err)
	h.lsm.mu.Lock()
	defer h.lsm.mu.Unlock()

	if info, ok := h.lsm.activeSessions[sessionID]; ok {
		info.IsStreaming = false
	}
	delete(h.lsm.tailCallbacks, sessionID)
}

// startFogLogStreamingLocked starts streaming fog logs (must be called with lock held)
func (lsm *LogSessionManager) startFogLogStreamingLocked(sessionID, iofogUUID string, tailConfig *utils.TailConfig) {
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Starting fog log streaming: sessionId=%s, iofogUuid=%s", sessionID, iofogUUID))

	lsm.stopLogStreamingLocked(sessionID)

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

	if tailConfig == nil {
		tailConfig = &utils.TailConfig{Follow: true, Lines: 100}
	}

	// Get log directory from config
	cfg := config.GetInstance()
	logDir := cfg.LogDirectory

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
	stopped   atomic.Bool
}

func (h *fogLogHandler) OnLogLine(_, _, line string) {
	if h.stopped.Load() || h.wsHandler == nil || line == "" {
		return
	}
	lineBytes := []byte(line)
	msgType := byte(6) // LogTypeLogLine
	_ = h.wsHandler.SendMessage(msgType, lineBytes)
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

// stopLogStreamingLocked stops active tail readers without removing the session (must be called with lock held).
func (lsm *LogSessionManager) stopLogStreamingLocked(sessionID string) {
	if reader, ok := lsm.localLogReaders[sessionID]; ok {
		reader.Stop()
		delete(lsm.localLogReaders, sessionID)
	}

	if handler, ok := lsm.tailCallbacks[sessionID]; ok {
		handler.stopped.Store(true)
		delete(lsm.tailCallbacks, sessionID)
	}

	if info, ok := lsm.activeSessions[sessionID]; ok {
		if info.tailCancel != nil {
			info.tailCancel()
			info.tailCancel = nil
		}
		info.IsStreaming = false
		info.streamingStartedAt = time.Time{}
	}
}

// HandleWebSocketTransportClose stops log tailing when the controller WebSocket drops.
// The session row is retained so HandleLogSessions can reconnect on the next poll.
func (lsm *LogSessionManager) HandleWebSocketTransportClose(sessionID string) {
	lsm.mu.Lock()
	defer lsm.mu.Unlock()
	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Stopping log streaming after WebSocket transport close: %s", sessionID))
	lsm.stopLogStreamingLocked(sessionID)
}

func (lsm *LogSessionManager) reconnectLogSessionLocked(sessionID string) {
	wsHandler := lsm.webSocketHandlers[sessionID]
	if wsHandler == nil {
		return
	}
	if wsHandler.IsConnected() {
		return
	}
	lsm.mu.Unlock()
	if wsHandler.needsResetBeforeConnect() {
		wsHandler.Reset()
	}
	err := wsHandler.Connect()
	lsm.mu.Lock()
	if err != nil {
		logging.LogError(logSessionManagerModuleName, fmt.Sprintf("Failed to reconnect log WebSocket: %s", sessionID), err)
	}
}

// stopLogSessionLocked stops a log session (must be called with lock held)
func (lsm *LogSessionManager) stopLogSessionLocked(sessionID string) {
	lsm.stopLogSessionWithReasonLocked(sessionID, logCloseReasonSessionStopped)
}

func (lsm *LogSessionManager) stopLogSessionWithReasonLocked(sessionID, closeReason string) {
	lsm.stopLogStreamingLocked(sessionID)

	logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Stopping log session: %s", sessionID))

	// Stop WebSocket handler
	if handler, exists := lsm.webSocketHandlers[sessionID]; exists {
		handler.DisconnectWithReason(closeReason)
		delete(lsm.webSocketHandlers, sessionID)
	}

	// Remove from active sessions
	delete(lsm.activeSessions, sessionID)

	logging.LogDebug(logSessionManagerModuleName, fmt.Sprintf("Stopped log session: %s", sessionID))
}

func (lsm *LogSessionManager) expireOldLogSessionsLocked() {
	now := time.Now()
	expired := make([]string, 0)
	for sessionID, info := range lsm.activeSessions {
		if !info.IsStreaming || info.streamingStartedAt.IsZero() {
			continue
		}
		if now.Sub(info.streamingStartedAt) >= logSessionMaxDuration {
			expired = append(expired, sessionID)
		}
	}
	for _, sessionID := range expired {
		logging.LogInfo(logSessionManagerModuleName, fmt.Sprintf("Log session max duration reached: %s", sessionID))
		lsm.stopLogSessionWithReasonLocked(sessionID, logCloseReasonMaxDuration)
	}
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
			lsm.mu.Lock()
			lsm.expireOldLogSessionsLocked()
			lsm.mu.Unlock()

			sessions, err := lsm.FetchLogSessions(ctx)
			if err != nil {
				logging.LogError(logSessionManagerModuleName, "Error fetching log sessions", err)
				continue
			}
			lsm.HandleLogSessions(sessions)
		}
	}
}
