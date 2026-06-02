package fieldagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/datasance/edgelet/internal/auth"
	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	logWebSocketModuleName = "Log Session WebSocket Handler"
	logPingInterval        = 30 * time.Second
	logHandshakeTimeout    = 10 * time.Second
	logMaxFrameSize        = 65536
	logMaxBufferSize       = 1024 * 1024 // 1MB
	logMaxBufferedFrames   = 1000
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
	controllerCert    *x509.Certificate
	logSessionManager *LogSessionManager // Reference to start tailing when ready
}

var (
	activeLogHandlers sync.Map // map[string]*LogSessionWebSocketHandler
)

// GetLogSessionWebSocketHandler gets or creates a LogSessionWebSocketHandler for a session
func GetLogSessionWebSocketHandler(sessionID, microserviceUUID, iofogUUID string, isMicroserviceLog bool) *LogSessionWebSocketHandler {
	key := sessionID
	handler, _ := activeLogHandlers.LoadOrStore(key, newLogSessionWebSocketHandler(sessionID, microserviceUUID, iofogUUID, isMicroserviceLog))
	return handler.(*LogSessionWebSocketHandler)
}

// newLogSessionWebSocketHandler creates a new LogSessionWebSocketHandler
func newLogSessionWebSocketHandler(sessionID, microserviceUUID, iofogUUID string, isMicroserviceLog bool) *LogSessionWebSocketHandler {
	cfg := config.GetInstance()
	controllerURL := cfg.ControllerURL
	if controllerURL == "" {
		logging.LogError(logWebSocketModuleName, "Controller URL is not configured", fmt.Errorf("controller URL is empty"))
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
		if cfg.ControllerCert != "" {
			cert, err := auth.LoadCertificateFromBase64(cfg.ControllerCert)
			if err != nil {
				// Try loading as PEM string directly
				cert, err = auth.LoadCertificateFromPEM([]byte(cfg.ControllerCert))
				if err != nil {
					logging.LogError(logWebSocketModuleName, "Failed to load controller certificate", err)
				} else {
					handler.controllerCert = cert
				}
			} else {
				handler.controllerCert = cert
			}
		}
	}

	return handler
}

// Connect establishes the WebSocket connection to the controller
func (h *LogSessionWebSocketHandler) Connect() error {
	h.connMu.Lock()
	defer h.connMu.Unlock()

	if h.isConnected.Load() {
		return nil
	}

	if !h.transitionState(LogStateDisconnected, LogStateConnecting) {
		return fmt.Errorf("cannot connect: invalid state transition")
	}

	// Generate JWT token
	token, err := h.jwtManager.GenerateJWT()
	if err != nil {
		h.transitionState(LogStateConnecting, LogStateDisconnected)
		return fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Create WebSocket dialer
	dialer := websocket.Dialer{
		HandshakeTimeout: logHandshakeTimeout,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: !h.config.SecureMode, // #nosec G402 -- controlled by SecureMode config; false in production
		},
	}

	// Add certificate to TLS config only if using WSS and certificate is available
	if strings.HasPrefix(strings.ToLower(h.controllerWsURL), "wss://") && h.controllerCert != nil {
		certPool := x509.NewCertPool()
		certPool.AddCert(h.controllerCert)
		dialer.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			RootCAs:            certPool,
			InsecureSkipVerify: !h.config.SecureMode, // #nosec G402 -- controlled by SecureMode config; false in production
		}
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
	// Transition to PENDING state after handshake
	// PENDING means connected but waiting for LOG_START message
	if h.transitionState(LogStateConnecting, LogStatePending) {
		logging.LogInfo(logWebSocketModuleName, "Connection is now pending LOG_START message")
	}

	// Start ping ticker
	h.pingTicker = time.NewTicker(logPingInterval)
	h.wg.Add(2)
	go h.pingWorker()
	go h.readWorker()

	// NO initial message for log sessions - wait for LOG_START from controller

	logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("WebSocket connection established for log session: %s", h.sessionID))
	return nil
}

// transitionState safely transitions connection state
func (h *LogSessionWebSocketHandler) transitionState(from, to LogConnectionState) bool {
	current := h.state.Load().(LogConnectionState)
	if current == from {
		h.state.Store(to)
		logging.LogDebug(logWebSocketModuleName, fmt.Sprintf("Connection state transition: %v -> %v", from, to))
		return true
	}
	return false
}

// SendMessage sends a log message to the controller
func (h *LogSessionWebSocketHandler) SendMessage(msgType byte, data []byte) error {
	if !h.isConnected.Load() {
		logging.LogWarn(logWebSocketModuleName, "Cannot send message - not connected")
		return fmt.Errorf("not connected")
	}

	// If not active, buffer the output
	if !h.isActive.Load() {
		logging.LogDebug(logWebSocketModuleName, fmt.Sprintf("Buffering log output while connection is not active: type=%d, length=%d", msgType, len(data)))

		// Check buffer limits
		if h.bufferedFrames.Load() >= logMaxBufferedFrames {
			logging.LogWarn(logWebSocketModuleName, "Buffer full, dropping frame")
			return fmt.Errorf("buffer full")
		}
		if h.bufferedSize.Load()+int64(len(data)) > logMaxBufferSize {
			logging.LogWarn(logWebSocketModuleName, "Buffer size limit reached, dropping frame")
			return fmt.Errorf("buffer size limit reached")
		}

		// Store both the type and data so flushBuffer can preserve the original msgType.
		bufferedData := make([]byte, len(data))
		copy(bufferedData, data)

		select {
		case h.outputBuffer <- logFrame{msgType: msgType, data: bufferedData}:
			h.bufferedFrames.Add(1)
			h.bufferedSize.Add(int64(len(data)))
		default:
			logging.LogWarn(logWebSocketModuleName, "Buffer channel full, dropping frame")
			return fmt.Errorf("buffer channel full")
		}
		return nil
	}

	// Pack message using MessagePack
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

			// Set read deadline
			if err := conn.SetReadDeadline(time.Now().Add(logPingInterval * 2)); err != nil {
				logging.LogWarn(logWebSocketModuleName, fmt.Sprintf("Failed to set read deadline: %v", err))
			}

			messageType, data, err := conn.ReadMessage()
			if err != nil {
				// Unified error handling: any error → close session, no reconnect.
				// Log appropriately: info for normal close (1000, 1001, 1005), error for others.
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, 1005) {
					logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("WebSocket closed: %v", err))
				} else {
					logging.LogError(logWebSocketModuleName, "WebSocket read error", err)
				}
				go h.handleClose()
				return
			}

			if messageType == websocket.BinaryMessage {
				h.handleMessage(data)
			} else if messageType == websocket.PongMessage {
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
		case "microserviceUuid", "iofogUuid", "timestamp":
			// Skip these fields (not needed for processing)
			err = dec.Skip()
			if err != nil {
				logging.LogError(logWebSocketModuleName, "Failed to skip value", err)
				return
			}
		default:
			// Skip unknown keys
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
		h.handleClose()
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
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		logging.LogError(logWebSocketModuleName, "Failed to parse LOG_START data as JSON", err)
		return
	}

	tailConfigMap, ok := config["tailConfig"].(map[string]interface{})
	if !ok {
		logging.LogWarn(logWebSocketModuleName, "LOG_START message missing tailConfig")
	}

	logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Received LOG_START message with tailConfig: sessionId=%s", sessionID))
	// Transition from PENDING to ACTIVE
	if h.transitionState(LogStatePending, LogStateActive) {
		h.isActive.Store(true)

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
			if err := h.SendMessage(frame.msgType, frame.data); err != nil {
				logging.LogError(logWebSocketModuleName, "Failed to send buffered message", err)
			} else {
				h.bufferedFrames.Add(-1)
				h.bufferedSize.Add(-int64(len(frame.data)))
			}
		default:
			return
		}
	}
}

// Disconnect closes the WebSocket connection
func (h *LogSessionWebSocketHandler) Disconnect() {
	h.cancel()

	h.connMu.Lock()
	if h.conn != nil {
		if err := h.conn.Close(); err != nil {
			logging.LogWarn(logWebSocketModuleName, fmt.Sprintf("Failed to close connection on disconnect: %v", err))
		}
		h.conn = nil
	}
	h.connMu.Unlock()

	h.isConnected.Store(false)
	h.isActive.Store(false)
	h.transitionState(LogStateConnected, LogStateDisconnected)

	h.wg.Wait()

	if h.pingTicker != nil {
		h.pingTicker.Stop()
	}

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

// handleClose handles WebSocket close
func (h *LogSessionWebSocketHandler) handleClose() {
	if !h.isConnected.Load() {
		logging.LogDebug(logWebSocketModuleName, fmt.Sprintf("Already disconnected for session: %s", h.sessionID))
		return
	}

	logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Handling close for session: %s, connectionState=%v",
		h.sessionID, h.state.Load()))

	h.isConnected.Store(false)
	h.isActive.Store(false)
	h.state.Store(LogStateDisconnected)
	// Cleanup connection
	h.Disconnect()

	logging.LogInfo(logWebSocketModuleName, fmt.Sprintf("Close handling completed for session: %s", h.sessionID))
}
