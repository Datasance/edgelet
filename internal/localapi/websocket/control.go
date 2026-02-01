package websocket

import (
	"encoding/binary"
	"net/http"
	"strings"

	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/gorilla/websocket"
)

const (
	controlHandlerModuleName = "Control WebSocket Handler"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// Check origin against configured allowed origins
			// For now, allow localhost and same-origin requests
			origin := r.Header.Get("Origin")
			if origin == "" {
				// No origin header, allow (same-origin or non-browser client)
				return true
			}

			// Allow localhost origins
			if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") {
				return true
			}

			// In production, you might want to check against a whitelist
			// For now, allow all origins (can be restricted later)
			return true
		},
	}
)

// ControlHandler handles control WebSocket connections
type ControlHandler struct {
	manager *Manager
}

// NewControlHandler creates a new control WebSocket handler
func NewControlHandler() *ControlHandler {
	return &ControlHandler{
		manager: GetManager(),
	}
}

// Handle handles the WebSocket upgrade and connection
func (h *ControlHandler) Handle(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(controlHandlerModuleName, "Open the websocket for the real-time control signals")

	// Extract container ID from URL path
	// Expected format: /v2/control/socket/id/{containerId}
	id, err := extractIDFromPath(r.URL.Path, "/v2/control/socket/id/")
	if err != nil {
		logging.LogError(controlHandlerModuleName, "Missing ID or ID value in URL", err)
		http.Error(w, "Missing ID or ID value in URL", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.LogError(controlHandlerModuleName, "Failed to upgrade connection", err)
		return
	}

	// Register connection
	connection := h.manager.AddConnection(ControlWebSocket, id, conn)
	logging.LogDebug(controlHandlerModuleName, "Websocket for the real-time control signals is open")

	// Handle connection in goroutine
	go h.handleConnection(connection)
}

// handleConnection handles messages from a WebSocket connection
func (h *ControlHandler) handleConnection(conn *Connection) {
	defer func() {
		conn.Conn.Close()
		h.manager.RemoveConnection(ControlWebSocket, conn.ID)
		logging.LogDebug(controlHandlerModuleName, "Control WebSocket connection closed")
	}()

	for {
		messageType, message, err := conn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logging.LogError(controlHandlerModuleName, "WebSocket error", err)
			}
			break
		}

		if messageType == websocket.BinaryMessage {
			h.handleBinaryMessage(conn, message)
		} else if messageType == websocket.PingMessage {
			h.handlePing(conn)
		} else if messageType == websocket.CloseMessage {
			break
		}
	}
}

// handleBinaryMessage handles binary WebSocket messages
func (h *ControlHandler) handleBinaryMessage(conn *Connection, data []byte) {
	if len(data) < 1 {
		return
	}

	opcode := data[0]

	switch opcode {
	case OpcodePing:
		// Respond with pong
		pongData := []byte{OpcodePong}
		if err := conn.Conn.WriteMessage(websocket.BinaryMessage, pongData); err != nil {
			logging.LogError(controlHandlerModuleName, "Failed to send pong", err)
		}
	case OpcodeACK:
		// Acknowledge received
		// TODO: Remove from unacknowledged map when implemented (acknowledgment tracking)
		logging.LogDebug(controlHandlerModuleName, "ACK received")
	default:
		logging.LogDebug(controlHandlerModuleName, "Unknown opcode")
	}
}

// handlePing handles ping messages
func (h *ControlHandler) handlePing(conn *Connection) {
	// Respond with pong
	pongData := []byte{OpcodePong}
	if err := conn.Conn.WriteMessage(websocket.PongMessage, pongData); err != nil {
		logging.LogError(controlHandlerModuleName, "Failed to send pong", err)
	}
}

// SendControlSignal sends a control signal to a specific container
func (h *ControlHandler) SendControlSignal(containerID string) error {
	conn, exists := h.manager.GetConnection(ControlWebSocket, containerID)
	if !exists {
		return nil // Connection doesn't exist, nothing to do
	}

	signalData := []byte{OpcodeControlSignal}
	return conn.Conn.WriteMessage(websocket.BinaryMessage, signalData)
}

// SendControlSignalToAll sends a control signal to all connected containers
func (h *ControlHandler) SendControlSignalToAll(changedConfigIDs []string) {
	allConnections := h.manager.GetAllConnections(ControlWebSocket)

	for _, containerID := range changedConfigIDs {
		if conn, exists := allConnections[containerID]; exists {
			signalData := []byte{OpcodeControlSignal}
			if err := conn.Conn.WriteMessage(websocket.BinaryMessage, signalData); err != nil {
				logging.LogError(controlHandlerModuleName, "Failed to send control signal", err)
			}
		}
	}
}

// SendResourceSignal sends a resource signal to all connected containers
func (h *ControlHandler) SendResourceSignal() {
	allConnections := h.manager.GetAllConnections(ControlWebSocket)

	for _, conn := range allConnections {
		signalData := []byte{OpcodeResourceSignal}
		if err := conn.Conn.WriteMessage(websocket.BinaryMessage, signalData); err != nil {
			logging.LogError(controlHandlerModuleName, "Failed to send resource signal", err)
		}
	}
}

// Helper function to convert int to bytes (big-endian)
func intToBytes(value int) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(value))
	return buf
}

// Helper function to convert bytes to int (big-endian)
func bytesToInt(data []byte) int {
	if len(data) < 4 {
		return 0
	}
	return int(binary.BigEndian.Uint32(data))
}

// Helper function to convert long to bytes (big-endian)
func longToBytes(value int64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(value))
	return buf
}

// Helper function to convert bytes to long (big-endian)
func bytesToLong(data []byte) int64 {
	if len(data) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(data))
}
