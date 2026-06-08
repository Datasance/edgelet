package models

import "time"

// LogMessageType represents the type of log message
const (
	LogMessageTypeLogLine  byte = 6
	LogMessageTypeLogStart byte = 7
	LogMessageTypeLogStop  byte = 8
	LogMessageTypeLogError byte = 9
)

// LogMessage represents a message for log session WebSocket communication
type LogMessage struct {
	Type             byte   `json:"type" yaml:"type"` // 6: LOG_LINE, 7: LOG_START, 8: LOG_STOP, 9: LOG_ERROR
	Data             []byte `json:"data" yaml:"data"`
	SessionID        string `json:"sessionId" yaml:"sessionId"`
	MicroserviceUUID string `json:"microserviceUuid,omitempty" yaml:"microserviceUuid,omitempty"` // For microservice logs
	IofogUUID        string `json:"iofogUuid,omitempty" yaml:"iofogUuid,omitempty"`               // For fog logs
	Timestamp        int64  `json:"timestamp" yaml:"timestamp"`
}

// NewLogMessage creates a new LogMessage
func NewLogMessage(msgType byte, data []byte, sessionID, microserviceUUID, iofogUUID string) *LogMessage {
	return &LogMessage{
		Type:             msgType,
		Data:             data,
		SessionID:        sessionID,
		MicroserviceUUID: microserviceUUID,
		IofogUUID:        iofogUUID,
		Timestamp:        time.Now().UnixMilli(),
	}
}
