package messagebus

import (
	"context"
	"sync"

	"github.com/Azure/go-amqp"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	publisherModuleName = "MessagePublisher"
)

// MessagePublisher publishes messages to AMQP
type MessagePublisher struct {
	name      string
	route     *models.Route
	producers []*amqp.Sender
	archive   *MessageArchive
	mu        sync.Mutex
	logger    *logging.ModuleLogger
}

// NewMessagePublisher creates a new MessagePublisher
func NewMessagePublisher(name string, route *models.Route, producers []*amqp.Sender) *MessagePublisher {
	return &MessagePublisher{
		name:      name,
		route:     route,
		producers: producers,
		archive:   NewMessageArchive(name),
		logger:    logging.NewModuleLogger(publisherModuleName),
	}
}

// GetName returns the publisher name
func (p *MessagePublisher) GetName() string {
	return p.name
}

// GetRoute returns the current route
func (p *MessagePublisher) GetRoute() *models.Route {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.route
}

// Publish publishes a message
func (p *MessagePublisher) Publish(message *models.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.logger.Debugf("Start publish message: %s", p.name)

	// Serialize message to binary format
	messageBytes, err := message.GetBytes()
	if err != nil {
		return err
	}

	// Save to archive
	if err := p.archive.Save(messageBytes, message.Timestamp); err != nil {
		p.logger.Errorf("Message Publisher (%s) unable to archive message: %v", p.name, err)
		// Continue even if archive fails
	}

	// Publish to all receivers via AMQP
	for _, producer := range p.producers {
		if producer == nil {
			continue
		}

		// Create AMQP message with binary data
		amqpMsg := &amqp.Message{
			Data: [][]byte{messageBytes},
		}

		// Send message (non-persistent, default priority, no TTL)
		if err := producer.Send(context.Background(), amqpMsg, nil); err != nil {
			p.logger.Errorf("Message Publisher (%s) unable to send message: %v", p.name, err)
			// Continue to other receivers even if one fails
		}
	}

	p.logger.Debugf("Finished publish message: %s", p.name)
	return nil
}

// UpdateRoute updates the route and producers
func (p *MessagePublisher) UpdateRoute(route *models.Route, producers []*amqp.Sender) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.logger.Debug("Updating route")
	p.route = route
	p.producers = producers
}

// MessageQuery queries messages from the archive within a time range
func (p *MessagePublisher) MessageQuery(from, to int64) []*models.Message {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.archive == nil {
		return []*models.Message{}
	}

	return p.archive.MessageQuery(from, to)
}

// Close closes the publisher and archive
func (p *MessagePublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.logger.Debugf("Closing publisher: %s", p.name)

	// Close archive
	if p.archive != nil {
		if err := p.archive.Close(); err != nil {
			p.logger.Errorf("Error closing archive: %v", err)
		}
	}

	// Note: Producers are managed by AMQPClient, so we don't close them here
	return nil
}
