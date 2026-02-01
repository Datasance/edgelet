package messagebus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/go-amqp"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	amqpClientModuleName = "AMQP Client"
)

// AMQPClient handles AMQP connection and channel management
type AMQPClient struct {
	conn        *amqp.Conn
	session     *amqp.Session
	consumers   map[string]*amqp.Receiver
	producers   map[string][]*amqp.Sender
	mu          sync.RWMutex
	logger      *logging.ModuleLogger
	routerHost  string
	routerPort  int
	caCert      string
	tlsCert     string
	tlsKey      string
	isConnected bool
	reconnectCh chan struct{}
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewAMQPClient creates a new AMQP client
func NewAMQPClient(routerHost string, routerPort int, caCert, tlsCert, tlsKey string) *AMQPClient {
	return &AMQPClient{
		consumers:   make(map[string]*amqp.Receiver),
		producers:   make(map[string][]*amqp.Sender),
		logger:      logging.NewModuleLogger(amqpClientModuleName),
		routerHost:  routerHost,
		routerPort:  routerPort,
		caCert:      caCert,
		tlsCert:     tlsCert,
		tlsKey:      tlsKey,
		reconnectCh: make(chan struct{}, 1),
		stopChan:    make(chan struct{}),
	}
}

// Connect establishes connection to AMQP broker
func (c *AMQPClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected {
		return nil
	}

	c.logger.Debug("Starting AMQP connection")

	// Build connection URL
	url := fmt.Sprintf("amqps://%s:%d", c.routerHost, c.routerPort)

	// Setup TLS config
	tlsConfig, err := c.setupTLS()
	if err != nil {
		return fmt.Errorf("failed to setup TLS: %w", err)
	}

	// Create connection options
	opts := &amqp.ConnOptions{
		TLSConfig: tlsConfig,
	}

	// Connect to broker
	conn, err := amqp.Dial(context.Background(), url, opts)
	if err != nil {
		return fmt.Errorf("failed to dial AMQP broker: %w", err)
	}

	// Create session
	session, err := conn.NewSession(context.Background(), nil)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create session: %w", err)
	}

	c.conn = conn
	c.session = session
	c.isConnected = true

	c.logger.Debug("AMQP connection established")

	// Start connection monitor
	c.wg.Add(1)
	go c.monitorConnection()

	return nil
}

// setupTLS configures TLS for the AMQP connection
func (c *AMQPClient) setupTLS() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Setup CA certificate for trust
	if c.caCert != "" {
		caCertPEM, err := base64.StdEncoding.DecodeString(c.caCert)
		if err != nil {
			return nil, fmt.Errorf("failed to decode CA cert: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCertPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
	}

	// Setup client certificate if provided
	if c.tlsCert != "" && c.tlsKey != "" {
		certPEM, err := base64.StdEncoding.DecodeString(c.tlsCert)
		if err != nil {
			return nil, fmt.Errorf("failed to decode TLS cert: %w", err)
		}

		keyPEM, err := base64.StdEncoding.DecodeString(c.tlsKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode TLS key: %w", err)
		}

		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to create key pair: %w", err)
		}

		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// monitorConnection monitors the connection and handles reconnection
func (c *AMQPClient) monitorConnection() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopChan:
			return
		case <-c.reconnectCh:
			c.logger.Warn("Reconnection requested")
			c.reconnect()
		case <-time.After(30 * time.Second):
			// Periodic health check
			if !c.isConnected {
				c.reconnect()
			}
		}
	}
}

// reconnect attempts to reconnect to the broker
func (c *AMQPClient) reconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected {
		return
	}

	c.logger.Info("Attempting to reconnect to AMQP broker")

	// Close existing connection if any
	if c.conn != nil {
		c.conn.Close()
	}

	// Wait before reconnecting
	time.Sleep(2 * time.Second)

	// Try to reconnect
	if err := c.Connect(); err != nil {
		c.logger.Errorf("Reconnection failed: %v", err)
		// Schedule another reconnection attempt
		go func() {
			time.Sleep(5 * time.Second)
			select {
			case c.reconnectCh <- struct{}{}:
			default:
			}
		}()
	}
}

// CreateConsumer creates a new consumer for a receiver
func (c *AMQPClient) CreateConsumer(receiverName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected || c.session == nil {
		return fmt.Errorf("not connected to AMQP broker")
	}

	// Check if consumer already exists
	if _, exists := c.consumers[receiverName]; exists {
		return nil
	}

	c.logger.Debugf("Creating consumer for receiver: %s", receiverName)

	// Create receiver (consumer)
	receiver, err := c.session.NewReceiver(context.Background(), receiverName, nil)
	if err != nil {
		return fmt.Errorf("failed to create receiver: %w", err)
	}

	c.consumers[receiverName] = receiver
	c.logger.Debugf("Consumer created for receiver: %s", receiverName)

	return nil
}

// GetConsumer returns the consumer for a receiver, creating it if needed
func (c *AMQPClient) GetConsumer(receiverName string) (*amqp.Receiver, error) {
	c.mu.RLock()
	consumer, exists := c.consumers[receiverName]
	c.mu.RUnlock()

	if exists && consumer != nil {
		return consumer, nil
	}

	// Create consumer if it doesn't exist
	if err := c.CreateConsumer(receiverName); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.consumers[receiverName], nil
}

// RemoveConsumer removes a consumer
func (c *AMQPClient) RemoveConsumer(receiverName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	consumer, exists := c.consumers[receiverName]
	if !exists {
		return nil
	}

	c.logger.Debugf("Removing consumer for receiver: %s", receiverName)

	if err := consumer.Close(context.Background()); err != nil {
		c.logger.Errorf("Error closing consumer: %v", err)
	}

	delete(c.consumers, receiverName)
	return nil
}

// CreateProducer creates producers for a publisher to send to multiple receivers
func (c *AMQPClient) CreateProducer(publisherName string, receivers []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected || c.session == nil {
		return fmt.Errorf("not connected to AMQP broker")
	}

	c.logger.Debugf("Creating producers for publisher: %s to %d receivers", publisherName, len(receivers))

	senders := make([]*amqp.Sender, 0, len(receivers))
	for _, receiverName := range receivers {
		sender, err := c.session.NewSender(context.Background(), receiverName, nil)
		if err != nil {
			// Close already created senders on error
			for _, s := range senders {
				s.Close(context.Background())
			}
			return fmt.Errorf("failed to create sender for receiver %s: %w", receiverName, err)
		}
		senders = append(senders, sender)
	}

	c.producers[publisherName] = senders
	c.logger.Debugf("Producers created for publisher: %s", publisherName)

	return nil
}

// GetProducer returns the producers for a publisher, creating them if needed
func (c *AMQPClient) GetProducer(publisherName string, receivers []string) ([]*amqp.Sender, error) {
	c.mu.RLock()
	producers, exists := c.producers[publisherName]
	c.mu.RUnlock()

	if exists && len(producers) > 0 {
		// Check if receivers match (simplified - in production might need to verify)
		return producers, nil
	}

	// Create producers if they don't exist
	if err := c.CreateProducer(publisherName, receivers); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.producers[publisherName], nil
}

// RemoveProducer removes producers for a publisher
func (c *AMQPClient) RemoveProducer(publisherName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	producers, exists := c.producers[publisherName]
	if !exists {
		return
	}

	c.logger.Debugf("Removing producers for publisher: %s", publisherName)

	for _, producer := range producers {
		if err := producer.Close(context.Background()); err != nil {
			c.logger.Errorf("Error closing producer: %v", err)
		}
	}

	delete(c.producers, publisherName)
}

// IsConnected returns whether the client is connected
func (c *AMQPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isConnected
}

// SetConnected sets the connection status
func (c *AMQPClient) SetConnected(connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isConnected = connected
}

// Close closes all connections and cleans up resources
func (c *AMQPClient) Close() error {
	close(c.stopChan)
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Close all consumers
	for name, consumer := range c.consumers {
		if err := consumer.Close(context.Background()); err != nil {
			c.logger.Errorf("Error closing consumer %s: %v", name, err)
		}
	}
	c.consumers = make(map[string]*amqp.Receiver)

	// Close all producers
	for name, producers := range c.producers {
		for _, producer := range producers {
			if err := producer.Close(context.Background()); err != nil {
				c.logger.Errorf("Error closing producer %s: %v", name, err)
			}
		}
	}
	c.producers = make(map[string][]*amqp.Sender)

	// Close session
	if c.session != nil {
		c.session.Close(context.Background())
		c.session = nil
	}

	// Close connection
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.isConnected = false
	return nil
}
