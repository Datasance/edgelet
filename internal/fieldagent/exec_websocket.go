package fieldagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/auth"
	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	execWebSocketModuleName = "Exec Session WebSocket Handler"
	maxReconnectAttempts    = 5
	reconnectDelay          = 5 * time.Second
	pingInterval            = 30 * time.Second
	handshakeTimeout        = 10 * time.Second
	maxFrameSize            = 65536
	maxBufferSize           = 1024 * 1024 // 1MB
	maxBufferedFrames       = 1000
)

// ExecMessageType constants
const (
	ExecTypeStdin      byte = 0
	ExecTypeStdout     byte = 1
	ExecTypeStderr     byte = 2
	ExecTypeControl    byte = 3
	ExecTypeClose      byte = 4
	ExecTypeActivation byte = 5
)

// ConnectionState represents the WebSocket connection state
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StatePending // Connected but waiting for user
	StateActive  // Connected and paired with user
)

// ExecSessionWebSocketHandler manages the WebSocket connection for exec sessions
type ExecSessionWebSocketHandler struct {
	controllerWsURL   string
	microserviceUUID  string
	conn              *websocket.Conn
	connMu            sync.RWMutex
	writeMu           sync.Mutex // Protects concurrent writes to websocket
	isConnected       atomic.Bool
	isActive          atomic.Bool
	state             atomic.Value // ConnectionState
	reconnectAttempts int
	outputBuffer      chan []byte
	bufferedSize      atomic.Int64
	bufferedFrames    atomic.Int32
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	pingTicker        *time.Ticker
	config            *config.Config
	jwtManager        *auth.JWTManager
	controllerCert    *x509.Certificate
}

var (
	activeExecHandlers sync.Map // map[string]*ExecSessionWebSocketHandler
)

// GetExecSessionWebSocketHandler gets or creates an ExecSessionWebSocketHandler for a microservice
func GetExecSessionWebSocketHandler(microserviceUUID string) *ExecSessionWebSocketHandler {
	handler, _ := activeExecHandlers.LoadOrStore(microserviceUUID, newExecSessionWebSocketHandler(microserviceUUID))
	return handler.(*ExecSessionWebSocketHandler)
}

// newExecSessionWebSocketHandler creates a new ExecSessionWebSocketHandler
func newExecSessionWebSocketHandler(microserviceUUID string) *ExecSessionWebSocketHandler {
	cfg := config.GetInstance()
	controllerURL := cfg.ControllerURL
	if controllerURL == "" {
		logging.LogError(execWebSocketModuleName, "Controller URL is not configured", fmt.Errorf("controller URL is empty"))
		return nil
	}

	// Convert HTTP/HTTPS URL to WebSocket URL
	wsURL := convertToWebSocketURL(controllerURL)
	controllerWsURL := wsURL + "/agent/exec/" + microserviceUUID

	ctx, cancel := context.WithCancel(context.Background())

	handler := &ExecSessionWebSocketHandler{
		controllerWsURL:  controllerWsURL,
		microserviceUUID: microserviceUUID,
		outputBuffer:     make(chan []byte, maxBufferedFrames),
		ctx:              ctx,
		cancel:           cancel,
		config:           cfg,
		jwtManager:       auth.GetJWTManager(),
	}

	handler.state.Store(StateDisconnected)
	handler.isConnected.Store(false)
	handler.isActive.Store(false)

	// Load controller certificate only if using WSS (secure WebSocket)
	// Matching Java: only load cert if controllerWsUrl.startsWith("wss")
	if strings.HasPrefix(strings.ToLower(controllerWsURL), "wss://") {
		if cfg.ControllerCert != "" {
			cert, err := auth.LoadCertificateFromBase64(cfg.ControllerCert)
			if err != nil {
				// Try loading as PEM string directly
				cert, err = auth.LoadCertificateFromPEM([]byte(cfg.ControllerCert))
				if err != nil {
					logging.LogError(execWebSocketModuleName, "Failed to load controller certificate", err)
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

// convertToWebSocketURL converts HTTP/HTTPS URL to WebSocket URL
func convertToWebSocketURL(httpURL string) string {
	if strings.HasPrefix(httpURL, "http://") {
		return "ws://" + httpURL[7:]
	} else if strings.HasPrefix(httpURL, "https://") {
		return "wss://" + httpURL[8:]
	}
	// If already a WebSocket URL, return as is
	return httpURL
}

// Connect establishes the WebSocket connection to the controller
func (h *ExecSessionWebSocketHandler) Connect() error {
	h.connMu.Lock()

	// Idempotency check: if already connected or connecting, do nothing (matching Java)
	if h.isConnected.Load() {
		h.connMu.Unlock()
		logging.LogDebug(execWebSocketModuleName, "Connection already established, skipping Connect")
		return nil
	}

	currentState := h.state.Load().(ConnectionState)
	if currentState == StateConnecting || currentState == StatePending || currentState == StateActive {
		h.connMu.Unlock()
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Connection in progress or established (state=%v), skipping Connect", currentState))
		return nil
	}

	// Scoped lock for connection establishment
	err := func() error {
		defer h.connMu.Unlock()

		if !h.transitionState(StateDisconnected, StateConnecting) {
			logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("Cannot transition from %v to CONNECTING", currentState))
			return fmt.Errorf("cannot connect: invalid state transition")
		}

		// Generate JWT token
		token, err := h.jwtManager.GenerateJWT()
		if err != nil {
			h.transitionState(StateConnecting, StateDisconnected)
			return fmt.Errorf("failed to generate JWT: %w", err)
		}

		// Create WebSocket dialer
		dialer := websocket.Dialer{
			HandshakeTimeout: handshakeTimeout,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !h.config.SecureMode,
			},
		}

		// Add certificate to TLS config only if using WSS and certificate is available
		// Matching Java: only add SSL handler if controllerWsUrl.startsWith("wss") && sslContext != null
		if strings.HasPrefix(strings.ToLower(h.controllerWsURL), "wss://") && h.controllerCert != nil {
			certPool := x509.NewCertPool()
			certPool.AddCert(h.controllerCert)
			dialer.TLSClientConfig = &tls.Config{
				RootCAs:            certPool,
				InsecureSkipVerify: !h.config.SecureMode,
			}
		}

		// Set headers with JWT token
		headers := http.Header{}
		headers.Set("Authorization", "Bearer "+token)

		// Connect
		conn, resp, err := dialer.Dial(h.controllerWsURL, headers)
		if err != nil {
			h.transitionState(StateConnecting, StateDisconnected)
			if resp != nil {
				resp.Body.Close()
			}
			return fmt.Errorf("failed to connect: %w", err)
		}
		if resp != nil {
			resp.Body.Close()
		}

		h.conn = conn
		h.isConnected.Store(true)
		h.reconnectAttempts = 0
		return nil
	}()

	if err != nil {
		return err
	}

	// Lock is released here, proceeding with post-connection setup

	// Transition to PENDING state after handshake (matching Java line 320: CONNECTING -> PENDING)
	// PENDING means connected but waiting for user activation
	if h.transitionState(StateConnecting, StatePending) {
		logging.LogInfo(execWebSocketModuleName, "Connection is now pending user activation")
	}

	// Start ping ticker
	h.pingTicker = time.NewTicker(pingInterval)
	h.wg.Add(2)
	go h.pingWorker()
	go h.readWorker()

	// Send initial message (matching Java line 323: sendInitialMessage() after transition to PENDING)
	// Send immediately - execId should already be stored before Connect() is called
	// Safe to call now as we released the lock
	h.sendInitialMessage()

	logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("WebSocket connection established for microservice: %s", h.microserviceUUID))
	return nil
}

// transitionState safely transitions connection state
func (h *ExecSessionWebSocketHandler) transitionState(from, to ConnectionState) bool {
	current := h.state.Load().(ConnectionState)
	if current == from {
		h.state.Store(to)
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Connection state transition: %v -> %v", from, to))
		return true
	}
	return false
}

// sendInitialMessage sends the initial message with execId and microserviceUuid
func (h *ExecSessionWebSocketHandler) sendInitialMessage() {
	if !h.isConnected.Load() {
		logging.LogWarn(execWebSocketModuleName, "Cannot send initial message - not connected")
		return
	}

	// Get execId from FieldAgent (matching Java line 205)
	fa := GetInstance()
	execID := fa.GetActiveExecSession(h.microserviceUUID)
	if execID == "" {
		logging.LogError(execWebSocketModuleName, fmt.Sprintf("No execId found for microservice: %s - this may be a timing issue, will retry", h.microserviceUUID), fmt.Errorf("execId not found"))
		// Retry after a short delay (execId might not be stored yet)
		go func() {
			time.Sleep(500 * time.Millisecond)
			if h.isConnected.Load() {
				retryExecID := fa.GetActiveExecSession(h.microserviceUUID)
				if retryExecID != "" {
					logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Retrying initial message send with execId: %s", retryExecID))
					h.sendInitialMessageWithExecID(retryExecID)
				} else {
					logging.LogError(execWebSocketModuleName, fmt.Sprintf("Still no execId found after retry for microservice: %s", h.microserviceUUID), fmt.Errorf("execId not found"))
				}
			}
		}()
		return
	}

	h.sendInitialMessageWithExecID(execID)
}

// sendInitialMessageWithExecID sends the initial message with a specific execId
func (h *ExecSessionWebSocketHandler) sendInitialMessageWithExecID(execID string) {

	// Pack message using MessagePack
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)

	// Pack as map with 2 key-value pairs
	err := enc.EncodeMapLen(2)
	if err != nil {
		logging.LogError(execWebSocketModuleName, "Failed to encode map length", err)
		return
	}

	// Pack execId
	err = enc.EncodeString("execId")
	if err != nil {
		logging.LogError(execWebSocketModuleName, "Failed to encode execId key", err)
		return
	}
	err = enc.EncodeString(execID)
	if err != nil {
		logging.LogError(execWebSocketModuleName, "Failed to encode execId value", err)
		return
	}

	// Pack microserviceUuid
	err = enc.EncodeString("microserviceUuid")
	if err != nil {
		logging.LogError(execWebSocketModuleName, "Failed to encode microserviceUuid key", err)
		return
	}
	err = enc.EncodeString(h.microserviceUUID)
	if err != nil {
		logging.LogError(execWebSocketModuleName, "Failed to encode microserviceUuid value", err)
		return
	}

	// Send as binary WebSocket frame
	h.connMu.RLock()
	conn := h.conn
	h.connMu.RUnlock()

	if conn != nil {
		h.writeMu.Lock()
		defer h.writeMu.Unlock()
		msgBytes := buf.Bytes()
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Sending initial message: execId=%s, microserviceUuid=%s, messageLength=%d", execID, h.microserviceUUID, len(msgBytes)))
		err = conn.WriteMessage(websocket.BinaryMessage, msgBytes)
		if err != nil {
			logging.LogError(execWebSocketModuleName, "Failed to send initial message", err)
		} else {
			logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Sent initial message successfully for microservice: %s", h.microserviceUUID))
		}
	}
}

// SendMessage sends a message to the controller
func (h *ExecSessionWebSocketHandler) SendMessage(msgType byte, data []byte) error {
	if !h.isConnected.Load() {
		logging.LogWarn(execWebSocketModuleName, "Cannot send message - not connected")
		return fmt.Errorf("not connected")
	}

	// If not active, buffer the output
	if !h.isActive.Load() {
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Buffering output while connection is not active: type=%d, length=%d", msgType, len(data)))

		// Check buffer limits
		if h.bufferedFrames.Load() >= maxBufferedFrames {
			logging.LogWarn(execWebSocketModuleName, "Buffer full, dropping frame")
			return fmt.Errorf("buffer full")
		}
		if h.bufferedSize.Load()+int64(len(data)) > maxBufferSize {
			logging.LogWarn(execWebSocketModuleName, "Buffer size limit reached, dropping frame")
			return fmt.Errorf("buffer size limit reached")
		}

		// Create a copy of data for buffering
		bufferedData := make([]byte, len(data))
		copy(bufferedData, data)

		select {
		case h.outputBuffer <- bufferedData:
			h.bufferedFrames.Add(1)
			h.bufferedSize.Add(int64(len(data)))
		default:
			logging.LogWarn(execWebSocketModuleName, "Buffer channel full, dropping frame")
			return fmt.Errorf("buffer channel full")
		}
		return nil
	}

	// Get execId from FieldAgent
	fa := GetInstance()
	execID := fa.GetActiveExecSession(h.microserviceUUID)
	if execID == "" {
		logging.LogError(execWebSocketModuleName, fmt.Sprintf("No execId found for microservice: %s", h.microserviceUUID), fmt.Errorf("execId not found"))
		return fmt.Errorf("execId not found")
	}

	// Pack message using MessagePack
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)

	// Pack as map with 5 key-value pairs
	err := enc.EncodeMapLen(5)
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

	// Microservice UUID
	err = enc.EncodeString("microserviceUuid")
	if err != nil {
		return fmt.Errorf("failed to encode microserviceUuid key: %w", err)
	}
	err = enc.EncodeString(h.microserviceUUID)
	if err != nil {
		return fmt.Errorf("failed to encode microserviceUuid value: %w", err)
	}

	// Exec ID
	err = enc.EncodeString("execId")
	if err != nil {
		return fmt.Errorf("failed to encode execId key: %w", err)
	}
	err = enc.EncodeString(execID)
	if err != nil {
		return fmt.Errorf("failed to encode execId value: %w", err)
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
func (h *ExecSessionWebSocketHandler) pingWorker() {
	defer h.wg.Done()
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
						logging.LogError(execWebSocketModuleName, "Failed to send ping", err)
						h.handleConnectionFailure()
					}
				}
			}
		}
	}
}

// readWorker reads messages from the WebSocket
func (h *ExecSessionWebSocketHandler) readWorker() {
	defer h.wg.Done()

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
			conn.SetReadDeadline(time.Now().Add(pingInterval * 2))

			messageType, data, err := conn.ReadMessage()
			if err != nil {
				// Check for normal closure or "going away" (server restart/stop)
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					logging.LogInfo(execWebSocketModuleName, "WebSocket closed normally by server")
					h.handleClose()
					return
				}

				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logging.LogError(execWebSocketModuleName, "WebSocket read error", err)
				}
				h.handleConnectionFailure()
				return
			}

			// Add verbose logging for received messages
			logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("ReadMessage returned: type=%d, len=%d", messageType, len(data)))
			if len(data) > 0 {
				limit := 16
				if len(data) < limit {
					limit = len(data)
				}
				logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Raw data header: %x", data[:limit]))
			}

			if messageType == websocket.BinaryMessage {
				logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Received binary message from controller: length=%d", len(data)))
				h.handleMessage(data)
			} else if messageType == websocket.PongMessage {
				// Handle pong
				logging.LogDebug(execWebSocketModuleName, "Received pong")
			} else {
				logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Received message type: %d", messageType))
			}
		}
	}
}

// handleMessage processes incoming messages (matching Java: handleMessage())
func (h *ExecSessionWebSocketHandler) handleMessage(data []byte) {
	logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Received binary message: length=%d", len(data)))

	dec := msgpack.NewDecoder(bytes.NewReader(data))

	// Decode map
	mapLen, err := dec.DecodeMapLen()
	if err != nil {
		logging.LogError(execWebSocketModuleName, "Failed to decode map length", err)
		return
	}

	logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Unpacked map size: %d", mapLen))

	var msgType byte
	var msgData []byte
	var msgMicroserviceUUID string
	var msgExecID string

	// Decode all fields (matching Java: reads all key-value pairs)
	for i := 0; i < mapLen; i++ {
		key, err := dec.DecodeString()
		if err != nil {
			logging.LogError(execWebSocketModuleName, "Failed to decode key", err)
			return
		}

		switch key {
		case "type":
			val, err := dec.DecodeUint8()
			if err != nil {
				logging.LogError(execWebSocketModuleName, "Failed to decode type", err)
				return
			}
			msgType = val
			logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Unpacking key: %s, value: %d", key, msgType))
		case "data":
			val, err := dec.DecodeBytes()
			if err != nil {
				logging.LogError(execWebSocketModuleName, "Failed to decode data", err)
				return
			}
			msgData = val
		case "microserviceUuid":
			val, err := dec.DecodeString()
			if err != nil {
				logging.LogError(execWebSocketModuleName, "Failed to decode microserviceUuid", err)
				return
			}
			msgMicroserviceUUID = val
		case "execId":
			val, err := dec.DecodeString()
			if err != nil {
				logging.LogError(execWebSocketModuleName, "Failed to decode execId", err)
				return
			}
			msgExecID = val
		case "timestamp":
			// Skip timestamp (not needed for processing)
			err = dec.Skip()
			if err != nil {
				logging.LogError(execWebSocketModuleName, "Failed to skip timestamp", err)
				return
			}
		default:
			// Skip unknown keys
			err = dec.Skip()
			if err != nil {
				logging.LogError(execWebSocketModuleName, "Failed to skip value", err)
				return
			}
		}
	}

	logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Successfully unpacked message: type=%d, execId=%s, microserviceUuid=%s",
		msgType, msgExecID, msgMicroserviceUUID))

	// Handle message based on type (matching Java: handleMessage() switch)
	switch msgType {
	case ExecTypeStdin:
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Handling STDIN message: length=%d", len(msgData)))
		h.handleStdin(msgData)
	case ExecTypeControl:
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Handling CONTROL message: length=%d", len(msgData)))
		h.handleControl(msgData)
	case ExecTypeActivation:
		logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Handling ACTIVATION message: execId=%s, microserviceUuid=%s", msgExecID, msgMicroserviceUUID))
		h.handleActivation(msgMicroserviceUUID, msgExecID)
	case ExecTypeClose:
		logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Received close message for exec session: %s", msgExecID))
		h.handleClose()
	default:
		logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("Unknown message type: %d", msgType))
	}
}

// handleStdin handles STDIN messages (matching Java: handleStdin())
func (h *ExecSessionWebSocketHandler) handleStdin(data []byte) {
	if data == nil {
		return
	}

	fa := GetInstance()
	callback := fa.GetExecCallback(h.microserviceUUID)
	if callback == nil {
		logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("No active callback found for microservice: %s", h.microserviceUUID))
		return
	}

	err := callback.WriteInput(data)
	if err != nil {
		logging.LogError(execWebSocketModuleName, "Error writing input to exec callback", err)
	}
}

// handleControl handles CONTROL messages (matching Java: handleControl())
func (h *ExecSessionWebSocketHandler) handleControl(data []byte) {
	if data == nil {
		return
	}

	controlCmd := string(data)
	logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Handling CONTROL message: command=%s, length=%d, microserviceUuid=%s", controlCmd, len(data), h.microserviceUUID))

	if controlCmd == "close" {
		logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Received close control command for microservice: %s", h.microserviceUUID))
		h.handleClose()
	} else {
		logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("Unknown control command: %s", controlCmd))
	}
}

// handleActivation handles ACTIVATION messages (matching Java: handleActivation())
func (h *ExecSessionWebSocketHandler) handleActivation(microserviceUUID, execID string) {
	fa := GetInstance()
	currentExecID := fa.GetActiveExecSession(microserviceUUID)

	if currentExecID != "" && currentExecID == execID {
		logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Received activation message for exec session: %s", execID))
		// Transition from PENDING to ACTIVE (matching Java line 487)
		if h.transitionState(StatePending, StateActive) {
			h.isActive.Store(true)
			// Flush buffered output (matching Java line 490: flushBufferedOutput())
			go h.flushBuffer()
		}
	} else {
		logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("Received activation message for unknown exec session: %s, current: %s", execID, currentExecID))
	}
}

// flushBuffer sends all buffered messages
func (h *ExecSessionWebSocketHandler) flushBuffer() {
	for {
		select {
		case data := <-h.outputBuffer:
			// Determine message type from data (this is a simplification)
			// In practice, we'd need to track the type when buffering
			msgType := ExecTypeStdout // Default to stdout
			err := h.SendMessage(msgType, data)
			if err != nil {
				logging.LogError(execWebSocketModuleName, "Failed to send buffered message", err)
			} else {
				h.bufferedFrames.Add(-1)
				h.bufferedSize.Add(-int64(len(data)))
			}
		default:
			return
		}
	}
}

// handleConnectionFailure handles connection failures and attempts reconnection
func (h *ExecSessionWebSocketHandler) handleConnectionFailure() {
	h.connMu.Lock()
	if h.conn != nil {
		h.conn.Close()
		h.conn = nil
	}
	h.connMu.Unlock()

	h.isConnected.Store(false)
	h.transitionState(StateConnected, StateDisconnected)

	if h.reconnectAttempts < maxReconnectAttempts {
		h.reconnectAttempts++
		backoff := time.Duration(h.reconnectAttempts) * reconnectDelay
		logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Reconnecting in %v (attempt %d/%d)", backoff, h.reconnectAttempts, maxReconnectAttempts))

		time.Sleep(backoff)
		go h.Connect()
	} else {
		logging.LogError(execWebSocketModuleName, "Max reconnection attempts reached", fmt.Errorf("reconnection failed"))
	}
}

// Disconnect closes the WebSocket connection
func (h *ExecSessionWebSocketHandler) Disconnect() {
	h.cancel()

	h.connMu.Lock()
	if h.conn != nil {
		h.conn.Close()
		h.conn = nil
	}
	h.connMu.Unlock()

	h.isConnected.Store(false)
	h.isActive.Store(false)
	h.transitionState(StateConnected, StateDisconnected)

	h.wg.Wait()

	if h.pingTicker != nil {
		h.pingTicker.Stop()
	}

	logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Disconnected WebSocket for microservice: %s", h.microserviceUUID))
}

// IsConnected returns whether the WebSocket is connected
func (h *ExecSessionWebSocketHandler) IsConnected() bool {
	return h.isConnected.Load()
}

// IsActive returns whether the exec session is active
func (h *ExecSessionWebSocketHandler) IsActive() bool {
	return h.isActive.Load()
}

// handleClose handles WebSocket close (matching Java: handleClose())
func (h *ExecSessionWebSocketHandler) handleClose() {
	if !h.isConnected.Load() {
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Already disconnected for microservice: %s", h.microserviceUUID))
		return
	}

	logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Handling close for microservice: %s, connectionState=%v, reconnectAttempts=%d",
		h.microserviceUUID, h.state.Load(), h.reconnectAttempts))

	h.isConnected.Store(false)

	// Get current exec session ID before cleanup (matching Java line 576-579)
	fa := GetInstance()
	execID := fa.GetActiveExecSession(h.microserviceUUID)
	if execID != "" {
		// Coordinate with FieldAgent for exec session cleanup (matching Java line 579)
		fa.HandleExecSessionClose(h.microserviceUUID, execID)
	}

	// Check if there are other active exec sessions before cleanup (matching Java line 585-602)
	fa.execSessionsMu.RLock()
	hasOtherActiveSessions := fa.activeExecSessions[h.microserviceUUID] != ""
	fa.execSessionsMu.RUnlock()

	if !hasOtherActiveSessions {
		logging.LogDebug(execWebSocketModuleName, "No other active sessions found, proceeding with cleanup")
		// Cleanup connection (matching Java: cleanup())
		h.Disconnect()
	} else {
		logging.LogInfo(execWebSocketModuleName, "Skipping cleanup due to other active sessions")
	}

	logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Close handling completed for microservice: %s", h.microserviceUUID))
}
