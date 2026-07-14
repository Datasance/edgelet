package websocket

import "testing"

func TestRemoveConnection_OnlyRemovesMatchingConnection(t *testing.T) {
	m := &Manager{
		controlConnections: make(map[string]*Connection),
		messageConnections: make(map[string]*Connection),
	}

	stale := &Connection{ID: "ms-1"}
	replacement := &Connection{ID: "ms-1"}
	m.controlConnections["ms-1"] = replacement

	m.RemoveConnection(ControlWebSocket, "ms-1", stale)

	if replacement.IsClosed() {
		t.Fatal("stale remove must not close replacement connection")
	}
	if _, exists := m.GetConnection(ControlWebSocket, "ms-1"); !exists {
		t.Fatal("replacement connection should remain registered")
	}

	m.RemoveConnection(ControlWebSocket, "ms-1", replacement)

	if !replacement.IsClosed() {
		t.Fatal("matching remove should close connection")
	}
	if m.GetControlConnectionsCount() != 0 {
		t.Fatal("expected control connection map to be empty")
	}
}
