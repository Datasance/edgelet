package messagebus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/go-amqp"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	receiverModuleName = "MessageReceiver"
)

// MessageCallback is a callback function for real-time message receiving
type MessageCallback func(*models.Message)

// MessageReceiver receives messages from AMQP
type MessageReceiver struct {
	name           string
	consumer       *amqp.Receiver
	listener       MessageCallback
	realtimeMode   bool
	mu             sync.Mutex
	logger         *logging.ModuleLogger
	messageChannel chan *models.Message
	stopChan       chan struct{}
	wg             sync.WaitGroup
}

// NewMessageReceiver creates a new MessageReceiver
func NewMessageReceiver(name string, consumer *amqp.Receiver) *MessageReceiver {
	receiver := &MessageReceiver{
		name:           name,
		consumer:       consumer,
		logger:         logging.NewModuleLogger(receiverModuleName),
		messageChannel: make(chan *models.Message, 100),
		stopChan:       make(chan struct{}),
	}

	// Start message receiving goroutine
	receiver.wg.Add(1)
	go receiver.receiveLoop()

	return receiver
}

// GetName returns the receiver name
func (r *MessageReceiver) GetName() string {
	return r.name
}

// GetMessages returns all queued messages
func (r *MessageReceiver) GetMessages() ([]*models.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Debugf("Start getting message: %s", r.name)

	result := make([]*models.Message, 0)

	// If in real-time mode, return messages from channel
	if r.realtimeMode {
		// Drain channel
		for {
			select {
			case msg := <-r.messageChannel:
				result = append(result, msg)
			default:
				return result, nil
			}
		}
	}

	// Otherwise, receive messages directly from consumer
	if r.consumer == nil {
		return result, nil
	}

	// Receive messages with no-wait
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		msg, err := r.consumer.Receive(ctx, nil)
		cancel()

		if err != nil {
			// No more messages or timeout
			break
		}

		// Parse message
		message, err := r.parseMessage(msg)
		if err != nil {
			r.logger.Errorf("Error parsing message: %v", err)
			r.consumer.AcceptMessage(context.Background(), msg)
			continue
		}

		// Accept message
		if err := r.consumer.AcceptMessage(context.Background(), msg); err != nil {
			r.logger.Errorf("Error accepting message: %v", err)
		}

		result = append(result, message)
	}

	r.logger.Debugf("Finished getting message: %s", r.name)
	return result, nil
}

// receiveLoop continuously receives messages and handles them
func (r *MessageReceiver) receiveLoop() {
	defer r.wg.Done()

	for {
		select {
		case <-r.stopChan:
			return
		default:
			if r.consumer == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			msg, err := r.consumer.Receive(ctx, nil)
			cancel()

			if err != nil {
				// Timeout or error - continue
				continue
			}

			// Parse message
			message, err := r.parseMessage(msg)
			if err != nil {
				r.logger.Errorf("Error parsing message: %v", err)
				r.consumer.AcceptMessage(context.Background(), msg)
				continue
			}

			// Accept message
			if err := r.consumer.AcceptMessage(context.Background(), msg); err != nil {
				r.logger.Errorf("Error accepting message: %v", err)
				continue
			}

			// Handle message based on mode
			r.mu.Lock()
			realtimeMode := r.realtimeMode
			listener := r.listener
			r.mu.Unlock()

			if realtimeMode && listener != nil {
				// Real-time mode: call callback
				listener(message)
			} else {
				// Queue mode: add to channel
				select {
				case r.messageChannel <- message:
				default:
					// Channel full, drop message
					r.logger.Warn("Message channel full, dropping message")
				}
			}
		}
	}
}

// parseMessage parses an AMQP message to a models.Message
func (r *MessageReceiver) parseMessage(amqpMsg *amqp.Message) (*models.Message, error) {
	if len(amqpMsg.Data) == 0 {
		return nil, fmt.Errorf("empty message data")
	}

	// Parse JSON
	var message models.Message
	if err := json.Unmarshal(amqpMsg.Data[0], &message); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &message, nil
}

// EnableRealTimeReceiving enables real-time receiving mode
func (r *MessageReceiver) EnableRealTimeReceiving(callback MessageCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Debug("Start enable real time receiving")

	r.realtimeMode = true
	r.listener = callback

	r.logger.Debug("Finished enable real time receiving")
}

// DisableRealTimeReceiving disables real-time receiving mode
func (r *MessageReceiver) DisableRealTimeReceiving() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Debug("Start disable real time receiving")

	r.realtimeMode = false
	r.listener = nil

	r.logger.Debug("Finished disable real time receiving")
}

// Close closes the receiver
func (r *MessageReceiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Debugf("Start closing receiver: %s", r.name)

	// Stop receiving loop
	close(r.stopChan)
	r.wg.Wait()

	// Disable real-time receiving
	r.DisableRealTimeReceiving()

	// Close consumer (managed by AMQPClient, so we don't close it here)
	// Just clear the reference
	r.consumer = nil

	r.logger.Debugf("Finished closing receiver: %s", r.name)
	return nil
}
