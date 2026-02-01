package websocket

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/messagebus"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/gorilla/websocket"
)

const (
	messageHandlerModuleName = "Message WebSocket Handler"
)

// MessageHandler handles message WebSocket connections
type MessageHandler struct {
	manager *Manager
}

// NewMessageHandler creates a new message WebSocket handler
func NewMessageHandler() *MessageHandler {
	return &MessageHandler{
		manager: GetManager(),
	}
}

// Handle handles the WebSocket upgrade and connection
func (h *MessageHandler) Handle(w http.ResponseWriter, r *http.Request) {
	logging.LogInfo(messageHandlerModuleName, "Start Handler to open the websocket for the real-time message websocket")

	// Extract container ID from URL path
	// Expected format: /v2/message/socket/id/{containerId}
	id, err := extractIDFromPath(r.URL.Path, "/v2/message/socket/id/")
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Missing ID or ID value in URL", err)
		http.Error(w, "Missing ID or ID value in URL", http.StatusBadRequest)
		return
	}

	// Remove query parameters if any
	if idx := strings.Index(id, "?"); idx != -1 {
		id = id[:idx]
	}
	id = strings.TrimSpace(id)

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to upgrade connection", err)
		return
	}

	// Register connection
	connection := h.manager.AddConnection(MessageWebSocket, id, conn)
	logging.LogInfo(messageHandlerModuleName, "Finished Handler to open the websocket for the real-time message websocket. Handshake end....")

	// Enable real-time receiving in MessageBus
	mb := messagebus.GetInstance()
	callback := func(msg *models.Message) {
		// Send message to container via WebSocket
		msgBytes, err := msg.MarshalJSON()
		if err != nil {
			logging.LogError(messageHandlerModuleName, "Error marshaling message for real-time", err)
			return
		}
		if err := h.SendRealTimeMessage(id, msgBytes); err != nil {
			logging.LogError(messageHandlerModuleName, "Error sending real-time message", err)
		}
	}
	mb.EnableRealTimeReceiving(id, callback)

	// Handle connection in goroutine
	go h.handleConnection(connection)
}

// handleConnection handles messages from a WebSocket connection
func (h *MessageHandler) handleConnection(conn *Connection) {
	defer func() {
		conn.Conn.Close()
		h.manager.RemoveConnection(MessageWebSocket, conn.ID)
		
		// Disable real-time receiving in MessageBus
		mb := messagebus.GetInstance()
		mb.DisableRealTimeReceiving(conn.ID)
		
		logging.LogDebug(messageHandlerModuleName, "Message WebSocket connection closed")
	}()

	for {
		messageType, message, err := conn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logging.LogError(messageHandlerModuleName, "WebSocket error", err)
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
func (h *MessageHandler) handleBinaryMessage(conn *Connection, data []byte) {
	if len(data) < 1 {
		return
	}

	opcode := data[0]

	switch opcode {
	case OpcodePing:
		// Respond with pong
		pongData := []byte{OpcodePong}
		if err := conn.Conn.WriteMessage(websocket.BinaryMessage, pongData); err != nil {
			logging.LogError(messageHandlerModuleName, "Failed to send pong", err)
		}
	case OpcodeMSG:
		// Handle incoming message from container
		h.handleIncomingMessage(conn, data)
	case OpcodeACK:
		// Acknowledge received
		// TODO: Remove from unacknowledged map when implemented (acknowledgment tracking)
		logging.LogDebug(messageHandlerModuleName, "ACK received")
	default:
		logging.LogDebug(messageHandlerModuleName, "Unknown opcode")
	}
}

// handleIncomingMessage handles an incoming message from a container
func (h *MessageHandler) handleIncomingMessage(conn *Connection, data []byte) {
	if len(data) < 5 {
		logging.LogError(messageHandlerModuleName, "Message too short", nil)
		return
	}

	// Extract message length (4 bytes after opcode)
	msgLength := bytesToInt(data[1:5])
	if len(data) < 5+msgLength {
		logging.LogError(messageHandlerModuleName, "Message length mismatch", nil)
		return
	}

	// Extract message bytes
	messageBytes := data[5 : 5+msgLength]

	// Parse message and publish to MessageBus
	// Note: Full binary parsing is TODO in models/message.go
	// For now, try to parse as JSON (if message is JSON-encoded)
	var message *models.Message
	var messageID string
	var timestamp int64
	
	// Try to parse as JSON first
	if err := json.Unmarshal(messageBytes, &message); err == nil && message != nil {
		// Successfully parsed as JSON
		if message.ID != nil {
			messageID = *message.ID
		}
		timestamp = message.Timestamp
	} else {
		// Binary format - create a basic message
		// TODO: Implement full binary parsing when binary format is implemented
		mb := messagebus.GetInstance()
		messageID = mb.GetNextId()
		timestamp = time.Now().UnixMilli()
		message = models.NewMessage()
		message.ID = &messageID
		message.Timestamp = timestamp
		message.ContentData = messageBytes
		logging.LogDebug(messageHandlerModuleName, "Received binary message, using basic parsing")
	}

	// Publish message to MessageBus
	// The container ID (conn.ID) is the publisher
	publisherID := conn.ID
	mb := messagebus.GetInstance()
	publisher := mb.GetPublisher(publisherID)
	if publisher != nil {
		if err := publisher.Publish(message); err != nil {
			logging.LogError(messageHandlerModuleName, "Error publishing message to MessageBus", err)
		} else {
			logging.LogDebug(messageHandlerModuleName, "Published message from container: "+conn.ID)
		}
	} else {
		logging.LogWarn(messageHandlerModuleName, "Publisher not found for container: "+publisherID)
	}

	// Send receipt with actual message ID and timestamp
	h.sendReceipt(conn, messageID, timestamp)
}

// sendReceipt sends a receipt for a published message
func (h *MessageHandler) sendReceipt(conn *Connection, messageID string, timestamp int64) {
	// Format: [opcode(1)] [idLength(1)] [timestampLength(1)] [id] [timestamp(8)]
	idBytes := []byte(messageID)
	idLength := byte(len(idBytes))
	timestampLength := byte(8)

	buf := new(bytes.Buffer)
	buf.WriteByte(OpcodeReceipt)
	buf.WriteByte(idLength)
	buf.WriteByte(timestampLength)
	buf.Write(idBytes)
	buf.Write(longToBytes(timestamp))

	if err := conn.Conn.WriteMessage(websocket.BinaryMessage, buf.Bytes()); err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to send receipt", err)
	}
}

// handlePing handles ping messages
func (h *MessageHandler) handlePing(conn *Connection) {
	// Respond with pong
	pongData := []byte{OpcodePong}
	if err := conn.Conn.WriteMessage(websocket.PongMessage, pongData); err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to send pong", err)
	}
}

// SendRealTimeMessage sends a real-time message to a container
func (h *MessageHandler) SendRealTimeMessage(receiverID string, messageBytes []byte) error {
	conn, exists := h.manager.GetConnection(MessageWebSocket, receiverID)
	if !exists {
		logging.LogError(messageHandlerModuleName, "No active real-time websocket found for "+receiverID, nil)
		return nil // Connection doesn't exist, nothing to do
	}

	// Format: [opcode(1)] [length(4)] [message]
	totalLength := len(messageBytes)
	buf := new(bytes.Buffer)
	buf.WriteByte(OpcodeMSG)
	buf.Write(intToBytes(totalLength))
	buf.Write(messageBytes)

	// TODO: Add to unacknowledged map when implemented (for acknowledgment tracking)
	return conn.Conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

