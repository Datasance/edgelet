package messagebus

import (
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/processmanager"
	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	moduleName = "Message Bus"
)

// MessageBus is the main message bus module
type MessageBus struct {
	amqpClient           *AMQPClient
	routes               map[string]*models.Route
	publishers           map[string]*MessagePublisher
	receivers            map[string]*MessageReceiver
	archive              map[string]*MessageArchive
	idGenerator          *MessageIdGenerator
	microserviceManager  processmanager.MicroserviceManagerInterface
	mu                   sync.RWMutex
	updateLock           sync.Mutex
	routerHost           string
	routerPort           int
	caCert               string
	tlsCert              string
	tlsKey               string
	logger               *logging.ModuleLogger
	stopChan             chan struct{}
	wg                   sync.WaitGroup
	lastSpeedTime        int64
	lastSpeedMessageCount int64
}

var (
	instance *MessageBus
	once     sync.Once
)

// GetInstance returns the singleton MessageBus instance
func GetInstance() *MessageBus {
	once.Do(func() {
		instance = &MessageBus{
			routes:      make(map[string]*models.Route),
			publishers:  make(map[string]*MessagePublisher),
			receivers:   make(map[string]*MessageReceiver),
			archive:     make(map[string]*MessageArchive),
			logger:      logging.NewModuleLogger(moduleName),
			stopChan:    make(chan struct{}),
		}
	})
	return instance
}

// Start starts the Message Bus module
func (mb *MessageBus) Start(microserviceManager processmanager.MicroserviceManagerInterface) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.logger.Info("Starting Message Bus module")

	mb.microserviceManager = microserviceManager

	// Get configuration
	cfg := config.GetInstance()
	mb.routerHost = cfg.RouterHost
	mb.routerPort = cfg.RouterPort
	mb.caCert = cfg.CACert
	mb.tlsCert = cfg.TLSCert
	mb.tlsKey = cfg.TLSKey

	// Create AMQP client
	mb.amqpClient = NewAMQPClient(mb.routerHost, mb.routerPort, mb.caCert, mb.tlsCert, mb.tlsKey)

	// Create ID generator
	mb.idGenerator = NewMessageIdGenerator()

	// Start server connection in background
	mb.wg.Add(1)
	go mb.startServer()

	return nil
}

// startServer starts the AMQP server connection
func (mb *MessageBus) startServer() {
	defer mb.wg.Done()

	for {
		select {
		case <-mb.stopChan:
			return
		default:
			if mb.amqpClient.IsConnected() {
				time.Sleep(5 * time.Second)
				continue
			}

			mb.updateLock.Lock()
			if mb.amqpClient.IsConnected() {
				mb.updateLock.Unlock()
				continue
			}

			mb.logger.Info("Connecting to AMQP broker")

			if err := mb.amqpClient.Connect(); err != nil {
				mb.logger.Errorf("Error starting message bus module: %v", err)
				mb.amqpClient.SetConnected(false)
				mb.updateLock.Unlock()
				time.Sleep(2 * time.Second)
				mb.Stop()
				continue
			}

			// Initialize publishers and receivers
			if err := mb.init(); err != nil {
				mb.logger.Errorf("Error initializing message bus: %v", err)
				mb.amqpClient.SetConnected(false)
				mb.updateLock.Unlock()
				time.Sleep(2 * time.Second)
				mb.Stop()
				continue
			}

			mb.amqpClient.SetConnected(true)
			mb.updateLock.Unlock()

			// Start periodic route updates
			mb.wg.Add(1)
			go mb.updateRoutesLoop()

			// Start speed calculation
			mb.wg.Add(1)
			go mb.calculateSpeedLoop()
		}
	}
}

// init initializes publishers and receivers
func (mb *MessageBus) init() error {
	mb.logger.Debug("Initializing message bus")

	// Update publishers and receivers based on current routes
	return mb.updatePublishersAndReceivers()
}

// updatePublishersAndReceivers updates publishers and receivers based on routes
func (mb *MessageBus) updatePublishersAndReceivers() error {
	mb.updateLock.Lock()
	defer mb.updateLock.Unlock()

	if mb.microserviceManager == nil {
		return nil
	}

	// Get routes from microservice manager
	// Note: We'll need to add GetRoutes() method to MicroserviceManagerInterface
	// For now, we'll get microservices and build routes from them
	microservices := mb.microserviceManager.GetLatestMicroservices()

	// Build routes map from microservices
	newRoutes := make(map[string]*models.Route)
	newPublishers := make(map[string]bool)
	newReceivers := make(map[string]bool)

	for _, ms := range microservices {
		if len(ms.Routes) > 0 {
			route := &models.Route{
				Receivers: ms.Routes,
			}
			newRoutes[ms.MicroserviceUUID] = route
			newPublishers[ms.MicroserviceUUID] = true
			for _, receiver := range ms.Routes {
				newReceivers[receiver] = true
			}
		}

		// Consumers are also receivers
		if ms.IsConsumer {
			newReceivers[ms.MicroserviceUUID] = true
		}
	}

	// Remove old publishers
	for key, publisher := range mb.publishers {
		if !newPublishers[key] {
			publisher.Close()
			mb.amqpClient.RemoveProducer(key)
			delete(mb.publishers, key)
		} else {
			// Check if route changed
			newRoute := newRoutes[key]
			currentRoute := publisher.GetRoute()
			if newRoute != nil && !currentRoute.Equals(newRoute) {
				mb.amqpClient.RemoveProducer(key)
				producers, err := mb.amqpClient.GetProducer(key, newRoute.Receivers)
				if err != nil {
					mb.logger.Errorf("Error getting producers for %s: %v", key, err)
					continue
				}
				publisher.UpdateRoute(newRoute, producers)
			}
		}
	}

	// Add new publishers
	for key, route := range newRoutes {
		if _, exists := mb.publishers[key]; !exists {
			producers, err := mb.amqpClient.GetProducer(key, route.Receivers)
			if err != nil {
				mb.logger.Errorf("Error getting producers for %s: %v", key, err)
				continue
			}
			publisher := NewMessagePublisher(key, route, producers)
			mb.publishers[key] = publisher
		}
	}

	// Remove old receivers
	for key, receiver := range mb.receivers {
		if !newReceivers[key] {
			receiver.Close()
			mb.amqpClient.RemoveConsumer(key)
			delete(mb.receivers, key)
		}
	}

	// Add new receivers
	for key := range newReceivers {
		if _, exists := mb.receivers[key]; !exists {
			consumer, err := mb.amqpClient.GetConsumer(key)
			if err != nil {
				mb.logger.Errorf("Error getting consumer for %s: %v", key, err)
				continue
			}
			receiver := NewMessageReceiver(key, consumer)
			mb.receivers[key] = receiver
		}
	}

	mb.routes = newRoutes
	return nil
}

// updateRoutesLoop periodically updates routes
func (mb *MessageBus) updateRoutesLoop() {
	defer mb.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-mb.stopChan:
			return
		case <-ticker.C:
			if err := mb.updatePublishersAndReceivers(); err != nil {
				mb.logger.Errorf("Error updating publishers and receivers: %v", err)
			}
		}
	}
}

// calculateSpeedLoop periodically calculates message processing speed
func (mb *MessageBus) calculateSpeedLoop() {
	defer mb.wg.Done()

	cfg := config.GetInstance()
	freqMinutes := cfg.SpeedCalculationFreqMinutes
	if freqMinutes == 0 {
		freqMinutes = 5 // Default to 5 minutes
	}

	ticker := time.NewTicker(time.Duration(freqMinutes) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-mb.stopChan:
			return
		case <-ticker.C:
			mb.calculateSpeed()
		}
	}
}

// calculateSpeed calculates message processing speed
func (mb *MessageBus) calculateSpeed() {
	mb.logger.Debug("Start calculating message processing speed")
	// TODO: Implement speed calculation
	// This would track published/received message counts over time
	mb.logger.Debug("Finished calculating message processing speed")
}

// Stop stops the Message Bus module
func (mb *MessageBus) Stop() {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.logger.Info("Stopping Message Bus module")

	close(mb.stopChan)
	mb.wg.Wait()

	// Close all receivers
	for name, receiver := range mb.receivers {
		if err := receiver.Close(); err != nil {
			mb.logger.Errorf("Error closing receiver %s: %v", name, err)
		}
	}
	mb.receivers = make(map[string]*MessageReceiver)

	// Close all publishers
	for name, publisher := range mb.publishers {
		if err := publisher.Close(); err != nil {
			mb.logger.Errorf("Error closing publisher %s: %v", name, err)
		}
	}
	mb.publishers = make(map[string]*MessagePublisher)

	// Close AMQP client
	if mb.amqpClient != nil {
		if err := mb.amqpClient.Close(); err != nil {
			mb.logger.Errorf("Error closing AMQP client: %v", err)
		}
	}

	// Close ID generator
	if mb.idGenerator != nil {
		mb.idGenerator.Close()
	}

	mb.logger.Info("Message Bus module stopped")
}

// MessageQuery queries messages from a publisher's archive for a specific receiver within a time range
func (mb *MessageBus) MessageQuery(publisherID, receiverID string, from, to int64) []*models.Message {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	mb.logger.Debugf("Starting message query: publisher=%s, receiver=%s, from=%d, to=%d", publisherID, receiverID, from, to)

	// Validate time range
	if to < from {
		mb.logger.Warn("Invalid time range: end < start")
		return nil
	}

	// Check if route exists and receiver is in the route
	route, exists := mb.routes[publisherID]
	if !exists || route == nil {
		mb.logger.Debugf("Route not found for publisher: %s", publisherID)
		return nil
	}

	// Check if receiver is in the route
	receiverInRoute := false
	for _, receiver := range route.Receivers {
		if receiver == receiverID {
			receiverInRoute = true
			break
		}
	}

	if !receiverInRoute {
		mb.logger.Debugf("Receiver %s not in route for publisher %s", receiverID, publisherID)
		return nil
	}

	// Get publisher
	publisher := mb.publishers[publisherID]
	if publisher == nil {
		mb.logger.Debugf("Publisher not found: %s", publisherID)
		return nil
	}

	// Query messages from publisher's archive
	messages := publisher.MessageQuery(from, to)
	mb.logger.Debugf("Finished message query: found %d messages", len(messages))
	return messages
}

// GetPublisher returns a publisher for a given microservice ID
func (mb *MessageBus) GetPublisher(publisherID string) *MessagePublisher {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.publishers[publisherID]
}

// GetReceiver returns a receiver for a given microservice ID
func (mb *MessageBus) GetReceiver(receiverID string) *MessageReceiver {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.receivers[receiverID]
}

// GetNextId returns the next generated message ID
func (mb *MessageBus) GetNextId() string {
	if mb.idGenerator == nil {
		return ""
	}
	return mb.idGenerator.GetNextId()
}

// GetRoutes returns the current routes
func (mb *MessageBus) GetRoutes() map[string]*models.Route {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.routes
}

// EnableRealTimeReceiving enables real-time receiving for a receiver
func (mb *MessageBus) EnableRealTimeReceiving(receiverID string, callback MessageCallback) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.logger.Debug("Starting enable real time receiving")
	receiver := mb.receivers[receiverID]
	if receiver == nil {
		return
	}
	receiver.EnableRealTimeReceiving(callback)
	mb.logger.Debug("Finishing enable real time receiving")
}

// DisableRealTimeReceiving disables real-time receiving for a receiver
func (mb *MessageBus) DisableRealTimeReceiving(receiverID string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.logger.Debug("Starting disable real time receiving")
	receiver := mb.receivers[receiverID]
	if receiver == nil {
		return
	}
	receiver.DisableRealTimeReceiving()
	mb.logger.Debug("Finishing disable real time receiving")
}

// GetModuleIndex returns the module index
func (mb *MessageBus) GetModuleIndex() int {
	return utils.MessageBus
}

// GetName returns the module name
func (mb *MessageBus) GetName() string {
	return moduleName
}
