package websocket

import (
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocketType represents the type of WebSocket connection
type WebSocketType byte

const (
	// ControlWebSocket is for control signals
	ControlWebSocket WebSocketType = 'C'
	// MessageWebSocket is for real-time messages
	MessageWebSocket WebSocketType = 'M'
)

// Connection represents a WebSocket connection with metadata
type Connection struct {
	Conn     *websocket.Conn
	ID       string
	Type     WebSocketType
	mu       sync.RWMutex
	closed   bool
}

// Manager manages WebSocket connections
type Manager struct {
	controlConnections map[string]*Connection
	messageConnections map[string]*Connection
	mu                 sync.RWMutex
}

var (
	instance *Manager
	once     sync.Once
)

// GetManager returns the singleton WebSocket manager
func GetManager() *Manager {
	once.Do(func() {
		instance = &Manager{
			controlConnections: make(map[string]*Connection),
			messageConnections: make(map[string]*Connection),
		}
	})
	return instance
}

// AddConnection adds a WebSocket connection
func (m *Manager) AddConnection(wsType WebSocketType, id string, conn *websocket.Conn) *Connection {
	m.mu.Lock()
	defer m.mu.Unlock()

	connection := &Connection{
		Conn:   conn,
		ID:     id,
		Type:   wsType,
		closed: false,
	}

	switch wsType {
	case ControlWebSocket:
		m.controlConnections[id] = connection
	case MessageWebSocket:
		m.messageConnections[id] = connection
	}

	return connection
}

// RemoveConnection removes a WebSocket connection
func (m *Manager) RemoveConnection(wsType WebSocketType, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch wsType {
	case ControlWebSocket:
		if conn, exists := m.controlConnections[id]; exists {
			conn.mu.Lock()
			conn.closed = true
			conn.mu.Unlock()
			delete(m.controlConnections, id)
		}
	case MessageWebSocket:
		if conn, exists := m.messageConnections[id]; exists {
			conn.mu.Lock()
			conn.closed = true
			conn.mu.Unlock()
			delete(m.messageConnections, id)
		}
	}
}

// GetConnection gets a connection by type and ID
func (m *Manager) GetConnection(wsType WebSocketType, id string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch wsType {
	case ControlWebSocket:
		conn, exists := m.controlConnections[id]
		return conn, exists
	case MessageWebSocket:
		conn, exists := m.messageConnections[id]
		return conn, exists
	default:
		return nil, false
	}
}

// GetAllConnections returns all connections of a given type
func (m *Manager) GetAllConnections(wsType WebSocketType) map[string]*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch wsType {
	case ControlWebSocket:
		result := make(map[string]*Connection)
		for k, v := range m.controlConnections {
			result[k] = v
		}
		return result
	case MessageWebSocket:
		result := make(map[string]*Connection)
		for k, v := range m.messageConnections {
			result[k] = v
		}
		return result
	default:
		return make(map[string]*Connection)
	}
}

// GetControlConnectionsCount returns the number of control connections
func (m *Manager) GetControlConnectionsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.controlConnections)
}

// GetMessageConnectionsCount returns the number of message connections
func (m *Manager) GetMessageConnectionsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messageConnections)
}

// IsClosed checks if a connection is closed
func (c *Connection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}
