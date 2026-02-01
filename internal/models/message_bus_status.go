package models

import (
	"encoding/json"
	"sync"
)

// MessageBusStatus represents the Message Bus status
type MessageBusStatus struct {
	mu                              sync.RWMutex
	ProcessedMessages               int64            `json:"processedMessages" yaml:"processedMessages"`                             // Total processed messages
	PublishedMessagesPerMicroservice map[string]int64 `json:"publishedMessagesPerMicroservice" yaml:"publishedMessagesPerMicroservice"` // Messages per microservice
	AverageSpeed                      float32          `json:"averageSpeed" yaml:"averageSpeed"`                                       // Average message processing speed
}

// NewMessageBusStatus creates a new MessageBusStatus
func NewMessageBusStatus() *MessageBusStatus {
	return &MessageBusStatus{
		PublishedMessagesPerMicroservice: make(map[string]int64),
	}
}

// IncreasePublishedMessagesPerMicroservice increases the message count for a microservice and returns the status for chaining
func (m *MessageBusStatus) IncreasePublishedMessagesPerMicroservice(microservice string) *MessageBusStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProcessedMessages++
	m.PublishedMessagesPerMicroservice[microservice]++
	return m
}

// SetAverageSpeed sets the average speed and returns the status for chaining
func (m *MessageBusStatus) SetAverageSpeed(speed float32) *MessageBusStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AverageSpeed = speed
	return m
}

// RemovePublishedMessagesPerMicroservice removes a microservice from the message count map
func (m *MessageBusStatus) RemovePublishedMessagesPerMicroservice(microservice string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.PublishedMessagesPerMicroservice, microservice)
}

// GetPublishedMessagesPerMicroservice returns the message count for a microservice
func (m *MessageBusStatus) GetPublishedMessagesPerMicroservice(microservice string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.PublishedMessagesPerMicroservice[microservice]
}

// GetJSONPublishedMessagesPerMicroservice returns the published messages per microservice as a JSON string
func (m *MessageBusStatus) GetJSONPublishedMessagesPerMicroservice() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	type MessageCountJSON struct {
		ID           string `json:"id"`
		MessageCount int64  `json:"messagecount"`
	}
	
	counts := make([]MessageCountJSON, 0, len(m.PublishedMessagesPerMicroservice))
	for id, count := range m.PublishedMessagesPerMicroservice {
		counts = append(counts, MessageCountJSON{
			ID:           id,
			MessageCount: count,
		})
	}
	
	jsonData, err := json.Marshal(counts)
	if err != nil {
		return "[]"
	}
	return string(jsonData)
}
