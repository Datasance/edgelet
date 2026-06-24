package fieldagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
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
	execWebSocketModuleName = "Exec Session WebSocket Handler"
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
	controllerWsURL  string
	microserviceUUID string
	conn             *websocket.Conn
	connMu           sync.RWMutex
	writeMu          sync.Mutex // Protects concurrent writes to websocket
	isConnected      atomic.Bool
	isActive         atomic.Bool
	state            atomic.Value // ConnectionState
	outputBuffer     chan []byte
	bufferedSize     atomic.Int64
	bufferedFrames   atomic.Int32
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	pingTicker       *time.Ticker
	config           *config.Config
	jwtManager       *auth.JWTManager
	controllerCert   *x509.Certificate
	lifecycleMu      sync.Mutex
}

var (
	activeExecHandlers sync.Map // map[string]*ExecSessionWebSocketHandler
)

// GetExecSessionWebSocketHandler gets or creates an ExecSessionWebSocketHandler for a microservice
func GetExecSessionWebSocketHandler(microserviceUUID string) *ExecSessionWebSocketHandler {
	handler, _ := activeExecHandlers.LoadOrStore(microserviceUUID, newExecSessionWebSocketHandler(microserviceUUID))
	h, ok := handler.(*ExecSessionWebSocketHandler)
	if !ok {
		return nil
	}
	return h
}

// newExecSessionWebSocketHandler creates a new ExecSessionWebSocketHandler
func newExecSessionWebSocketHandler(microserviceUUID string) *ExecSessionWebSocketHandler {
	cfg := config.GetInstance()
	controllerURL := cfg.ControllerURL
	if controllerURL == "" {
		logging.LogError(execWebSocketModuleName, "Controller URL is not configured", errors.New("controller URL is empty"))
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

// Reset tears down transport state and prepares the handler for a fresh Connect().
func (h *ExecSessionWebSocketHandler) Reset() {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	h.resetLocked()
}

func (h *ExecSessionWebSocketHandler) resetLocked() {
	if h.cancel != nil {
		h.cancel()
	}

	closeWebSocketConn(&h.connMu, &h.conn)
	stopSessionPingTicker(&h.pingTicker)

	h.wg.Wait()

	h.isConnected.Store(false)
	h.isActive.Store(false)
	h.state.Store(StateDisconnected)
	drainExecOutputBuffer(h)
	recreateHandlerContext(&h.ctx, &h.cancel)
}

// GetConnectionState returns the current connection state (for tests and diagnostics).
func (h *ExecSessionWebSocketHandler) GetConnectionState() ConnectionState {
	state, ok := h.state.Load().(ConnectionState)
	if !ok {
		return StateDisconnected
	}
	return state
}

// Connect establishes the WebSocket connection to the controller.
// Call Reset() before Connect() when starting a new exec session.
func (h *ExecSessionWebSocketHandler) Connect() error {
	if err := h.connectTransport(); err != nil {
		return err
	}

	h.pingTicker = time.NewTicker(pingInterval)
	h.wg.Add(2)
	go h.pingWorker()
	go h.readWorker()

	h.sendInitialMessage()

	logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("WebSocket connection established for microservice: %s", h.microserviceUUID))
	return nil
}

func (h *ExecSessionWebSocketHandler) connectTransport() error {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()

	if h.isConnected.Load() {
		return errors.New("already connected; call Reset() before Connect()")
	}

	// Clear stale state left when isConnected was cleared without resetting state (legacy bug).
	h.state.Store(StateDisconnected)

	if !h.transitionState(StateDisconnected, StateConnecting) {
		return fmt.Errorf("cannot connect: invalid state transition from %v", h.GetConnectionState())
	}

	h.connMu.Lock()
	defer h.connMu.Unlock()

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
		h.transitionState(StateConnecting, StateDisconnected)
		if resp != nil {
			if cerr := resp.Body.Close(); cerr != nil {
				logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("Failed to close response body: %v", cerr))
			}
		}
		return fmt.Errorf("failed to connect: %w", err)
	}
	if resp != nil {
		if cerr := resp.Body.Close(); cerr != nil {
			logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("Failed to close response body: %v", cerr))
		}
	}

	h.conn = conn
	h.isConnected.Store(true)

	if h.transitionState(StateConnecting, StatePending) {
		logging.LogInfo(execWebSocketModuleName, "Connection is now pending user activation")
	}

	return nil
}

// transitionState safely transitions connection state
func (h *ExecSessionWebSocketHandler) transitionState(from, to ConnectionState) bool {
	current, ok := h.state.Load().(ConnectionState)
	if !ok {
		return false
	}
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

	// Get execId from FieldAgent
	fa := GetInstance()
	execID := fa.GetActiveExecSession(h.microserviceUUID)
	if execID == "" {
		logging.LogError(execWebSocketModuleName, fmt.Sprintf("No execId found for microservice: %s - this may be a timing issue, will retry", h.microserviceUUID), errors.New("execId not found"))
		// Retry after a short delay (execId might not be stored yet)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logging.LogError(execWebSocketModuleName, "Panic recovered", fmt.Errorf("%v", r))
				}
			}()
			time.Sleep(500 * time.Millisecond)
			if h.isConnected.Load() {
				retryExecID := fa.GetActiveExecSession(h.microserviceUUID)
				if retryExecID != "" {
					logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Retrying initial message send with execId: %s", retryExecID))
					h.sendInitialMessageWithExecID(retryExecID)
				} else {
					logging.LogError(execWebSocketModuleName, fmt.Sprintf("Still no execId found after retry for microservice: %s", h.microserviceUUID), errors.New("execId not found"))
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
		return errors.New("not connected")
	}

	// If not active, buffer the output
	if !h.isActive.Load() {
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Buffering output while connection is not active: type=%d, length=%d", msgType, len(data)))

		// Check buffer limits
		if h.bufferedFrames.Load() >= maxBufferedFrames {
			logging.LogWarn(execWebSocketModuleName, "Buffer full, dropping frame")
			return errors.New("buffer full")
		}
		if h.bufferedSize.Load()+int64(len(data)) > maxBufferSize {
			logging.LogWarn(execWebSocketModuleName, "Buffer size limit reached, dropping frame")
			return errors.New("buffer size limit reached")
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
			return errors.New("buffer channel full")
		}
		return nil
	}

	// Get execId from FieldAgent
	fa := GetInstance()
	execID := fa.GetActiveExecSession(h.microserviceUUID)
	if execID == "" {
		logging.LogError(execWebSocketModuleName, fmt.Sprintf("No execId found for microservice: %s", h.microserviceUUID), errors.New("execId not found"))
		return errors.New("execId not found")
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
		if r := recover(); r != nil {
			logging.LogError(execWebSocketModuleName, "Panic recovered", fmt.Errorf("%v", r))
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
						logging.LogError(execWebSocketModuleName, "Failed to send ping", err)
						go h.handleClose()
					}
				}
			}
		}
	}
}

// readWorker reads messages from the WebSocket
func (h *ExecSessionWebSocketHandler) readWorker() {
	defer h.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(execWebSocketModuleName, "Panic recovered", fmt.Errorf("%v", r))
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
			if err := conn.SetReadDeadline(time.Now().Add(pingInterval * 2)); err != nil {
				logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("Failed to set read deadline: %v", err))
			}

			messageType, data, err := conn.ReadMessage()
			if err != nil {
				// Unified error handling: any error → close session, no reconnect.
				// Log appropriately: info for normal close (1000, 1001, 1005), error for others.
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, 1005) {
					logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("WebSocket closed: %v", err))
				} else {
					logging.LogError(execWebSocketModuleName, "WebSocket read error", err)
				}
				go h.handleClose()
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

			switch messageType {
			case websocket.BinaryMessage:
				logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Received binary message from controller: length=%d", len(data)))
				h.handleMessage(data)
			case websocket.PongMessage:
				// Handle pong
				logging.LogDebug(execWebSocketModuleName, "Received pong")
			default:
				logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Received message type: %d", messageType))
			}
		}
	}
}

// handleMessage processes incoming messages
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

	// Decode all fields
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

	// Handle message based on type
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

// handleStdin handles STDIN messages
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

// handleControl handles CONTROL messages
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

// handleActivation handles ACTIVATION messages
func (h *ExecSessionWebSocketHandler) handleActivation(microserviceUUID, execID string) {
	fa := GetInstance()
	currentExecID := fa.GetActiveExecSession(microserviceUUID)

	if currentExecID != "" && currentExecID == execID {
		logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Received activation message for exec session: %s", execID))
		// Transition from PENDING to ACTIVE
		if h.transitionState(StatePending, StateActive) {
			h.isActive.Store(true)
			// Flush buffered output
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

// Disconnect closes the WebSocket connection and resets handler state for reuse.
func (h *ExecSessionWebSocketHandler) Disconnect() {
	h.Reset()
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

// handleClose handles WebSocket close
func (h *ExecSessionWebSocketHandler) handleClose() {
	if !h.isConnected.Load() {
		logging.LogDebug(execWebSocketModuleName, fmt.Sprintf("Already disconnected for microservice: %s", h.microserviceUUID))
		return
	}

	logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Handling close for microservice: %s, connectionState=%v",
		h.microserviceUUID, h.state.Load()))

	h.isConnected.Store(false)
	h.isActive.Store(false)
	h.state.Store(StateDisconnected)

	// Get current exec session ID before cleanup
	fa := GetInstance()
	execID := fa.GetActiveExecSession(h.microserviceUUID)
	if execID != "" {
		// Coordinate with FieldAgent for exec session cleanup
		if err := fa.HandleExecSessionClose(h.microserviceUUID, execID); err != nil {
			logging.LogWarn(execWebSocketModuleName, fmt.Sprintf("Failed to close exec session: %v", err))
		}
	}

	// Check if there are other active exec sessions before cleanup
	fa.execSessionsMu.RLock()
	hasOtherActiveSessions := fa.activeExecSessions[h.microserviceUUID] != ""
	fa.execSessionsMu.RUnlock()

	if !hasOtherActiveSessions {
		logging.LogDebug(execWebSocketModuleName, "No other active sessions found, proceeding with cleanup")
		h.Reset()
	} else {
		logging.LogInfo(execWebSocketModuleName, "Skipping cleanup due to other active sessions")
	}

	logging.LogInfo(execWebSocketModuleName, fmt.Sprintf("Close handling completed for microservice: %s", h.microserviceUUID))
}
