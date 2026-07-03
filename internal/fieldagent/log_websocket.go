package fieldagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	logWebSocketModuleName       = "Log Session WebSocket Handler"
	logPingInterval              = 30 * time.Second
	logPendingReadTimeout        = 120 * time.Second // matches Controller logPendingTimeoutMs
	logHandshakeTimeout          = 10 * time.Second
	logCloseReasonSessionStopped = "session stopped"
	logCloseReasonMaxDuration    = "max session duration"
	logMaxFrameSize              = 65536
	logMaxBufferSize             = 1024 * 1024 // 1MB
	logMaxBufferedFrames         = 1000
)

// LogMessageType constants
const (
	LogTypeLogLine  byte = 6
	LogTypeLogStart byte = 7
	LogTypeLogStop  byte = 8
	LogTypeLogError byte = 9
)

// LogConnectionState represents the WebSocket connection state
type LogConnectionState int

const (
	LogStateDisconnected LogConnectionState = iota
	LogStateConnecting
	LogStateConnected
	LogStatePending // Connected but waiting for LOG_START
	LogStateActive  // Connected and streaming logs
)

// logFrame pairs a message type with its payload for type-safe buffering.
// Storing the type alongside the data ensures flushBuffer preserves the original
// message type (e.g. stderr vs stdout) rather than defaulting to LogTypeLogLine.
type logFrame struct {
	msgType byte
	data    []byte
}

// LogSessionWebSocketHandler manages the WebSocket connection for log sessions
type LogSessionWebSocketHandler struct {
	controllerWsURL   string
	sessionID         string
	microserviceUUID  string
	iofogUUID         string
	isMicroserviceLog bool
	conn              *websocket.Conn
	connMu            sync.RWMutex
	writeMu           sync.Mutex // Protects concurrent writes to websocket
	isConnected       atomic.Bool
	isActive          atomic.Bool
	state             atomic.Value // LogConnectionState
	outputBuffer      chan logFrame
	bufferedSize      atomic.Int64
	bufferedFrames    atomic.Int32
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	pingTicker        *time.Ticker
	config            *config.Config
	jwtManager        *auth.JWTManager
	controllerTLS     *tls.Config
	logSessionManager *LogSessionManager // Reference to start tailing when ready
	lifecycleMu       sync.Mutex
}

var (
	activeLogHandlers sync.Map // map[string]*LogSessionWebSocketHandler
)

// GetLogSessionWebSocketHandler gets or creates a LogSessionWebSocketHandler for a session
func GetLogSessionWebSocketHandler(sessionID, microserviceUUID, iofogUUID string, isMicroserviceLog bool) *LogSessionWebSocketHandler {
	key := sessionID
	handler, _ := activeLogHandlers.LoadOrStore(key, newLogSessionWebSocketHandler(sessionID, microserviceUUID, iofogUUID, isMicroserviceLog))
	h, ok := handler.(*LogSessionWebSocketHandler)
	if !ok {
		return nil
	}
	return h
}

// newLogSessionWebSocketHandler creates a new LogSessionWebSocketHandler
func newLogSessionWebSocketHandler(sessionID, microserviceUUID, iofogUUID string, isMicroserviceLog bool) *LogSessionWebSocketHandler {
	cfg := config.GetInstance()
	controllerURL := cfg.ControllerURL
	if controllerURL == "" {
		logging.LogError(logWebSocketModuleName, "Controller URL is not configured", errors.New("controller URL is empty"))
		return nil
	}

	// Convert HTTP/HTTPS URL to WebSocket URL
	wsURL := convertToWebSocketURL(controllerURL)
	var controllerWsURL string
	if isMicroserviceLog {
		controllerWsURL = wsURL + "/agent/logs/microservice/" + microserviceUUID + "/" + sessionID
	} else {
		controllerWsURL = wsURL + "/agent/logs/iofog/" + iofogUUID + "/" + sessionID
	}

	ctx, cancel := context.WithCancel(context.Background())

	handler := &LogSessionWebSocketHandler{
		controllerWsURL:   controllerWsURL,
		sessionID:         sessionID,
		microserviceUUID:  microserviceUUID,
		iofogUUID:         iofogUUID,
		isMicroserviceLog: isMicroserviceLog,
		outputBuffer:      make(chan logFrame, logMaxBufferedFrames),
		ctx:               ctx,
		cancel:            cancel,
		config:            cfg,
		jwtManager:        auth.GetJWTManager(),
	}

	handler.state.Store(LogStateDisconnected)
	handler.isConnected.Store(false)
	handler.isActive.Store(false)

	// Load controller certificate only if using WSS (secure WebSocket)
	if strings.HasPrefix(strings.ToLower(controllerWsURL), "wss://") {
		handler.controllerTLS = buildControllerTLSConfig(cfg.SecureMode, cfg.ControllerCert, logWebSocketModuleName)
	}

	return handler
}

// Reset tears down transport state and prepares the handler for a fresh Connect().
func (h *LogSessionWebSocketHandler) Reset() {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	h.resetLocked()
}

func (h *LogSessionWebSocketHandler) resetLocked() {
	if h.cancel != nil {
		h.cancel()
	}

	closeWebSocketConn(&h.connMu, &h.conn)
	h.wg.Wait()
	stopSessionPingTicker(&h.pingTicker)

	h.isConnected.Store(false)
	h.isActive.Store(false)
	h.state.Store(LogStateDisconnected)
	drainLogOutputBuffer(h)
	recreateHandlerContext(&h.ctx, &h.cancel)
}

// GetConnectionState returns the current connection state (for tests and diagnostics).
func (h *LogSessionWebSocketHandler) GetConnectionState() LogConnectionState {
	state, ok := h.state.Load().(LogConnectionState)
	if !ok {
		return LogStateDisconnected
	}
	return state
}

// Connect establishes the WebSocket connection to the controller.
// Call Reset() before Connect() when reusing a handler for a new session.
func (h *LogSessionWebSocketHandler) Connect() error {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()

	if err := h.connectTransportLocked(); err != nil {
		return err
	}

	h.pingTicker = time.NewTicker(logPingInterval)
	h.wg.Add(2)
	go h.pingWorker()
	go h.readWorker()

	logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("WebSocket connection established for log session: %s", h.sessionID))
	return nil
}

func (h *LogSessionWebSocketHandler) connectTransportLocked() error {
	if h.isConnected.Load() {
		return errors.New("already connected; call Reset() before Connect()")
	}

	h.state.Store(LogStateDisconnected)

	if !h.transitionState(LogStateDisconnected, LogStateConnecting) {
		return fmt.Errorf("cannot connect: invalid state transition from %v", h.GetConnectionState())
	}

	h.connMu.Lock()
	defer h.connMu.Unlock()

	// Generate JWT token
	token, err := h.jwtManager.GenerateJWT()
	if err != nil {
		h.transitionState(LogStateConnecting, LogStateDisconnected)
		return fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Create WebSocket dialer
	dialer := websocket.Dialer{
		HandshakeTimeout: logHandshakeTimeout,
		TLSClientConfig:  h.controllerTLS,
	}

	// Set headers with JWT token
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)

	// Connect
	conn, resp, err := dialer.Dial(h.controllerWsURL, headers)
	if err != nil {
		h.transitionState(LogStateConnecting, LogStateDisconnected)
		if resp != nil {
			if cerr := resp.Body.Close(); cerr != nil {
				logging.LogWarn(logWebSocketModuleName, fmt.Sprintf("Failed to close response body: %v", cerr))
			}
		}
		return fmt.Errorf("failed to connect: %w", err)
	}
	if resp != nil {
		if cerr := resp.Body.Close(); cerr != nil {
			logging.LogWarn(logWebSocketModuleName, fmt.Sprintf("Failed to close response body: %v", cerr))
		}
	}

	h.conn = conn
	h.isConnected.Store(true)
	h.configureReadKeepalive(conn)

	if h.transitionState(LogStateConnecting, LogStatePending) {
		logging.LogInfo(logWebSocketModuleName, "Connection is now pending LOG_START message")
	}

	return nil
}

// transitionState safely transitions connection state
func (h *LogSessionWebSocketHandler) transitionState(from, to LogConnectionState) bool {
	current, ok := h.state.Load().(LogConnectionState)
	if !ok {
		return false
	}
	if current == from {
		h.state.Store(to)
		logging.LogDebug(logWebSocketModuleName, fmt.Sprintf("Connection state transition: %v -> %v", from, to))
		return true
	}
	return false
}

// SendMessage sends a log message to the controller.
// Output is buffered while disconnected or pending activation (exec-session parity).
func (h *LogSessionWebSocketHandler) SendMessage(msgType byte, data []byte) error {
	if !h.isConnected.Load() || !h.isActive.Load() {
		logging.LogDebug(logWebSocketModuleName, fmt.Sprintf("Buffering log output while connection is not active: connected=%v active=%v type=%d length=%d",
			h.isConnected.Load(), h.isActive.Load(), msgType, len(data)))
		return h.bufferOutput(msgType, data)
	}
	return h.writeOutboundMessage(msgType, data)
}

func (h *LogSessionWebSocketHandler) bufferOutput(msgType byte, data []byte) error {
	if h.bufferedFrames.Load() >= logMaxBufferedFrames {
		logging.LogWarn(logWebSocketModuleName, "Buffer full, dropping frame")
		return errors.New("buffer full")
	}
	if h.bufferedSize.Load()+int64(len(data)) > logMaxBufferSize {
		logging.LogWarn(logWebSocketModuleName, "Buffer size limit reached, dropping frame")
		return errors.New("buffer size limit reached")
	}

	bufferedData := make([]byte, len(data))
	copy(bufferedData, data)

	select {
	case h.outputBuffer <- logFrame{msgType: msgType, data: bufferedData}:
		h.bufferedFrames.Add(1)
		h.bufferedSize.Add(int64(len(data)))
	default:
		logging.LogWarn(logWebSocketModuleName, "Buffer channel full, dropping frame")
		return errors.New("buffer channel full")
	}
	return nil
}

func (h *LogSessionWebSocketHandler) writeOutboundMessage(msgType byte, data []byte) error {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)

	// Pack as map with 6 key-value pairs
	err := enc.EncodeMapLen(6)
	if err != nil {
		return fmt.Errorf("failed to encode map length: %w", err)
	}

	// Type
	err = enc.EncodeString("type")
	if err != nil {
		return fmt.Errorf("failed to encode type key: %w", err)
	}
	err = enc.EncodeUint8(msgType)
	if err != nil {
		return fmt.Errorf("failed to encode type value: %w", err)
	}

	// Data
	err = enc.EncodeString("data")
	if err != nil {
		return fmt.Errorf("failed to encode data key: %w", err)
	}
	err = enc.EncodeBytes(data)
	if err != nil {
		return fmt.Errorf("failed to encode data value: %w", err)
	}

	// Session ID
	err = enc.EncodeString("sessionId")
	if err != nil {
		return fmt.Errorf("failed to encode sessionId key: %w", err)
	}
	err = enc.EncodeString(h.sessionID)
	if err != nil {
		return fmt.Errorf("failed to encode sessionId value: %w", err)
	}

	// Microservice UUID and Iofog UUID
	if h.isMicroserviceLog && h.microserviceUUID != "" {
		// Microservice log: include microserviceUuid, set iofogUuid to nil
		err = enc.EncodeString("microserviceUuid")
		if err != nil {
			return fmt.Errorf("failed to encode microserviceUuid key: %w", err)
		}
		err = enc.EncodeString(h.microserviceUUID)
		if err != nil {
			return fmt.Errorf("failed to encode microserviceUuid value: %w", err)
		}
		err = enc.EncodeString("iofogUuid")
		if err != nil {
			return fmt.Errorf("failed to encode iofogUuid key: %w", err)
		}
		err = enc.EncodeNil() // nil for microservice logs
		if err != nil {
			return fmt.Errorf("failed to encode iofogUuid nil: %w", err)
		}
	} else if !h.isMicroserviceLog && h.iofogUUID != "" {
		// Fog log: set microserviceUuid to nil, include iofogUuid
		err = enc.EncodeString("microserviceUuid")
		if err != nil {
			return fmt.Errorf("failed to encode microserviceUuid key: %w", err)
		}
		err = enc.EncodeNil() // nil for fog logs
		if err != nil {
			return fmt.Errorf("failed to encode microserviceUuid nil: %w", err)
		}
		err = enc.EncodeString("iofogUuid")
		if err != nil {
			return fmt.Errorf("failed to encode iofogUuid key: %w", err)
		}
		err = enc.EncodeString(h.iofogUUID)
		if err != nil {
			return fmt.Errorf("failed to encode iofogUuid value: %w", err)
		}
	} else {
		// Fallback: both nil (shouldn't happen, but handle gracefully)
		err = enc.EncodeString("microserviceUuid")
		if err != nil {
			return fmt.Errorf("failed to encode microserviceUuid key: %w", err)
		}
		err = enc.EncodeNil()
		if err != nil {
			return fmt.Errorf("failed to encode microserviceUuid nil: %w", err)
		}
		err = enc.EncodeString("iofogUuid")
		if err != nil {
			return fmt.Errorf("failed to encode iofogUuid key: %w", err)
		}
		err = enc.EncodeNil()
		if err != nil {
			return fmt.Errorf("failed to encode iofogUuid nil: %w", err)
		}
	}

	// Timestamp
	err = enc.EncodeString("timestamp")
	if err != nil {
		return fmt.Errorf("failed to encode timestamp key: %w", err)
	}
	err = enc.EncodeInt64(time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("failed to encode timestamp value: %w", err)
	}

	// Send as binary WebSocket frame
	h.connMu.RLock()
	conn := h.conn
	h.connMu.RUnlock()

	if conn != nil {
		h.writeMu.Lock()
		defer h.writeMu.Unlock()
		err = conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
		if err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}
	}

	return nil
}

func (h *LogSessionWebSocketHandler) readWaitDuration() time.Duration {
	if h.isActive.Load() {
		return 0
	}
	return logPendingReadTimeout
}

func (h *LogSessionWebSocketHandler) extendReadDeadline(conn *websocket.Conn) error {
	if conn == nil {
		return nil
	}
	if h.isActive.Load() {
		return conn.SetReadDeadline(time.Time{})
	}
	return conn.SetReadDeadline(time.Now().Add(logPendingReadTimeout))
}

func (h *LogSessionWebSocketHandler) configureReadKeepalive(conn *websocket.Conn) {
	if conn == nil {
		return
	}
	extend := func() error {
		return h.extendReadDeadline(conn)
	}
	if err := extend(); err != nil {
		logging.LogWarn(logWebSocketModuleName, fmt.Sprintf("Failed to set read deadline: %v", err))
	}
	conn.SetPongHandler(func(string) error {
		return extend()
	})
	conn.SetPingHandler(func(appData string) error {
		if err := extend(); err != nil {
			return err
		}
		h.writeMu.Lock()
		defer h.writeMu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})
}

func (h *LogSessionWebSocketHandler) isPendingReadTimeout(err error) bool {
	if h.isActive.Load() {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// pingWorker sends ping frames periodically
func (h *LogSessionWebSocketHandler) pingWorker() {
	defer h.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(logWebSocketModuleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()
	defer func() {
		if h.pingTicker != nil {
			h.pingTicker.Stop()
		}
	}()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-h.pingTicker.C:
			if h.isConnected.Load() {
				h.connMu.RLock()
				conn := h.conn
				h.connMu.RUnlock()

				if conn != nil {
					h.writeMu.Lock()
					err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
					h.writeMu.Unlock()
					if err != nil {
						logging.LogError(logWebSocketModuleName, "Failed to send ping", err)
						go h.handleClose()
					}
				}
			}
		}
	}
}

// readWorker reads messages from the WebSocket
func (h *LogSessionWebSocketHandler) readWorker() {
	defer h.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(logWebSocketModuleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()

	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			h.connMu.RLock()
			conn := h.conn
			h.connMu.RUnlock()

			if conn == nil {
				return
			}

			if err := h.extendReadDeadline(conn); err != nil {
				logging.LogWarn(logWebSocketModuleName, fmt.Sprintf("Failed to set read deadline: %v", err))
			}

			messageType, data, err := conn.ReadMessage()
			if err != nil {
				switch {
				case h.isPendingReadTimeout(err):
					logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Pending LOG_START timeout for log session: %s", h.sessionID))
				case websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, 1005):
					logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("WebSocket closed: %v", err))
				default:
					logging.LogError(logWebSocketModuleName, "WebSocket read error", err)
				}
				go h.handleClose()
				return
			}

			switch messageType {
			case websocket.BinaryMessage:
				h.handleMessage(data)
			case websocket.PongMessage:
				// Handle pong
				logging.LogDebug(logWebSocketModuleName, "Received pong")
			}
		}
	}
}

// handleMessage processes incoming messages
func (h *LogSessionWebSocketHandler) handleMessage(data []byte) {
	dec := msgpack.NewDecoder(bytes.NewReader(data))

	// Decode map
	mapLen, err := dec.DecodeMapLen()
	if err != nil {
		logging.LogError(logWebSocketModuleName, "Failed to decode map length", err)
		return
	}

	var msgType byte
	var msgData []byte
	var sessionID string

	// Decode all fields
	for i := 0; i < mapLen; i++ {
		key, err := dec.DecodeString()
		if err != nil {
			logging.LogError(logWebSocketModuleName, "Failed to decode key", err)
			return
		}

		switch key {
		case "type":
			val, err := dec.DecodeUint8()
			if err != nil {
				logging.LogError(logWebSocketModuleName, "Failed to decode type", err)
				return
			}
			msgType = val
		case "data":
			val, err := dec.DecodeBytes()
			if err != nil {
				logging.LogError(logWebSocketModuleName, "Failed to decode data", err)
				return
			}
			msgData = val
		case "sessionId":
			val, err := dec.DecodeString()
			if err != nil {
				logging.LogError(logWebSocketModuleName, "Failed to decode sessionId", err)
				return
			}
			sessionID = val
		default:
			// Skip unknown and unused fields (microserviceUuid, iofogUuid, timestamp, etc.)
			err = dec.Skip()
			if err != nil {
				logging.LogError(logWebSocketModuleName, "Failed to skip value", err)
				return
			}
		}
	}

	// Handle message based on type
	switch msgType {
	case LogTypeLogStart:
		h.handleLogStart(msgData, sessionID)
	case LogTypeLogStop:
		logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Received LOG_STOP message for session: %s", sessionID))
		h.handleSessionClose()
	case LogTypeLogError:
		h.handleLogError(msgData)
	default:
		logging.LogWarn(logWebSocketModuleName, fmt.Sprintf("Unknown message type: %d", msgType))
	}
}

// handleLogStart handles LOG_START message
func (h *LogSessionWebSocketHandler) handleLogStart(data []byte, sessionID string) {
	if data == nil {
		logging.LogWarn(logWebSocketModuleName, "LOG_START message has no data")
		return
	}

	// Parse tailConfig from data
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		logging.LogError(logWebSocketModuleName, "Failed to parse LOG_START data as JSON", err)
		return
	}

	tailConfigMap, ok := config["tailConfig"].(map[string]any)
	if !ok {
		logging.LogWarn(logWebSocketModuleName, "LOG_START message missing tailConfig")
	}

	logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Received LOG_START message with tailConfig: sessionId=%s", sessionID))
	// Transition from PENDING to ACTIVE
	if h.transitionState(LogStatePending, LogStateActive) {
		h.isActive.Store(true)

		h.connMu.RLock()
		conn := h.conn
		h.connMu.RUnlock()
		if err := h.extendReadDeadline(conn); err != nil {
			logging.LogWarn(logWebSocketModuleName, fmt.Sprintf("Failed to extend read deadline on LOG_START: %v", err))
		}

		// Trigger log streaming start in LogSessionManager with tailConfig from LOG_START
		if h.logSessionManager != nil {
			logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Triggering log streaming start on WebSocket activation: sessionId=%s", sessionID))
			h.logSessionManager.StartLogStreamingOnActivation(sessionID, tailConfigMap)
		} else {
			logging.LogWarn(logWebSocketModuleName, "LogSessionManager not set, cannot start log streaming")
		}

		// Flush buffered output
		go h.flushBuffer()
	}
}

// handleLogError handles LOG_ERROR message
func (h *LogSessionWebSocketHandler) handleLogError(data []byte) {
	if data != nil {
		errorMsg := string(data)
		logging.LogError(logWebSocketModuleName, fmt.Sprintf("Received LOG_ERROR: %s", errorMsg), errors.New(errorMsg))
	}
}

// SetLogSessionManager sets the LogSessionManager reference
func (h *LogSessionWebSocketHandler) SetLogSessionManager(lsm *LogSessionManager) {
	h.logSessionManager = lsm
}

// flushBuffer sends all buffered messages, preserving each frame's original message type.
func (h *LogSessionWebSocketHandler) flushBuffer() {
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(logWebSocketModuleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()
	for {
		select {
		case frame := <-h.outputBuffer:
			h.bufferedFrames.Add(-1)
			h.bufferedSize.Add(-int64(len(frame.data)))
			if err := h.writeOutboundMessage(frame.msgType, frame.data); err != nil {
				logging.LogError(logWebSocketModuleName, "Failed to send buffered message", err)
			}
		default:
			return
		}
	}
}

func (h *LogSessionWebSocketHandler) gracefulCloseTransportLocked(reason string) {
	h.connMu.Lock()
	defer h.connMu.Unlock()
	if h.conn == nil {
		return
	}
	h.writeMu.Lock()
	deadline := time.Now().Add(time.Second)
	_ = h.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason), deadline)
	h.writeMu.Unlock()
	_ = h.conn.Close()
	h.conn = nil
}

// Disconnect closes the WebSocket connection and resets handler state for reuse.
func (h *LogSessionWebSocketHandler) Disconnect() {
	h.DisconnectWithReason(logCloseReasonSessionStopped)
}

// DisconnectWithReason closes the WebSocket with a specific close reason before reset.
func (h *LogSessionWebSocketHandler) DisconnectWithReason(reason string) {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	h.gracefulCloseTransportLocked(reason)
	h.resetLocked()
	logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Disconnected WebSocket for log session: %s", h.sessionID))
}

// IsConnected returns whether the WebSocket is connected
func (h *LogSessionWebSocketHandler) IsConnected() bool {
	return h.isConnected.Load()
}

// IsActive returns whether the log session is active
func (h *LogSessionWebSocketHandler) IsActive() bool {
	return h.isActive.Load()
}

// needsResetBeforeConnect reports whether stale transport state must be torn down before Connect().
func (h *LogSessionWebSocketHandler) needsResetBeforeConnect() bool {
	if h == nil {
		return false
	}
	if h.IsConnected() {
		return true
	}
	return h.GetConnectionState() != LogStateDisconnected
}

func (h *LogSessionWebSocketHandler) handleTransportClose() {
	if !h.isConnected.Load() {
		logging.LogDebug(logWebSocketModuleName, fmt.Sprintf("Already disconnected for session: %s", h.sessionID))
		return
	}

	logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Transport closed for log session: %s (tail stopped; session retained for reconnect)", h.sessionID))

	h.isConnected.Store(false)
	h.isActive.Store(false)
	h.state.Store(LogStateDisconnected)
	if h.logSessionManager != nil {
		h.logSessionManager.HandleWebSocketTransportClose(h.sessionID)
	}
	h.Reset()
}

func (h *LogSessionWebSocketHandler) handleSessionClose() {
	sessionID := h.sessionID
	if h.logSessionManager != nil {
		go h.logSessionManager.StopLogSession(sessionID)
		return
	}
	h.handleTransportClose()
}

// handleClose handles unexpected WebSocket transport loss.
func (h *LogSessionWebSocketHandler) handleClose() {
	h.handleTransportClose()
}
