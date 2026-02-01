package localapi

import (
	"context"
	"sync"

	"github.com/eclipse-iofog/agent-go/internal/utils"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	localAPIModuleName = "Local API"
	defaultPort        = 54321
)

// LocalAPI is the main Local API module
type LocalAPI struct {
	server *Server
	mu     sync.RWMutex
}

var (
	instance *LocalAPI
	once     sync.Once
)

// GetInstance returns the singleton LocalAPI instance
func GetInstance() *LocalAPI {
	once.Do(func() {
		instance = &LocalAPI{}
	})
	return instance
}

// Start starts the Local API server
func (l *LocalAPI) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logging.LogInfo(localAPIModuleName, "Start local api server")

	// Create and configure server
	l.server = NewServer(defaultPort)

	// Start server in goroutine
	go func() {
		if err := l.server.Start(); err != nil {
			logging.LogError(localAPIModuleName, "Failed to start local api server", err)
		}
	}()

	logging.LogInfo(localAPIModuleName, "Local api server started")
	return nil
}

// Stop stops the Local API server
func (l *LocalAPI) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logging.LogInfo(localAPIModuleName, "Stopping local api server")

	if l.server != nil {
		return l.server.Shutdown(context.Background())
	}

	return nil
}

// Update is called when configuration changes
func (l *LocalAPI) Update() {
	logging.LogDebug(localAPIModuleName, "Start the real-time control signal when the configuration updated")
	// This will be implemented to trigger control signals
	// For now, just log
	logging.LogDebug(localAPIModuleName, "Finish the real-time control signal when the configuration updated")
}

// UpdateEdgeResource is called when edge resources are updated
func (l *LocalAPI) UpdateEdgeResource() {
	logging.LogDebug(localAPIModuleName, "Start the real-time control signal when the edge resources are updated")
	// This will be implemented to trigger resource signals
	// For now, just log
	logging.LogDebug(localAPIModuleName, "Finished the real-time control signal when the edge resources are updated")
}

// GetName returns the module name
func (l *LocalAPI) GetName() string {
	return localAPIModuleName
}

// GetModuleIndex returns the module index
func (l *LocalAPI) GetModuleIndex() int {
	return utils.LocalAPI
}
