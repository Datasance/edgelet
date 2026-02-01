package models

import "time"

// ExecMessageType represents the type of exec message
const (
	ExecMessageTypeStdin   byte = 0
	ExecMessageTypeStdout  byte = 1
	ExecMessageTypeStderr  byte = 2
	ExecMessageTypeControl byte = 3
)

// ExecMessage represents a message for exec session WebSocket communication
type ExecMessage struct {
	Type             byte   `json:"type" yaml:"type"` // 0: STDIN, 1: STDOUT, 2: STDERR, 3: CONTROL
	Data             []byte `json:"data" yaml:"data"`
	MicroserviceUUID string `json:"microserviceUuid" yaml:"microserviceUuid"`
	ExecID           string `json:"execId" yaml:"execId"`
	Timestamp        int64  `json:"timestamp" yaml:"timestamp"`
}

// NewExecMessage creates a new ExecMessage
func NewExecMessage(msgType byte, data []byte, microserviceUUID, execID string) *ExecMessage {
	return &ExecMessage{
		Type:             msgType,
		Data:             data,
		MicroserviceUUID: microserviceUUID,
		ExecID:           execID,
		Timestamp:        time.Now().UnixMilli(),
	}
}
