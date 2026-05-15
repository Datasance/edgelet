package localapi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent/internal/localapi/handlers"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	localAPIModuleName = "Local API"
	defaultPort        = 54321
	localAPIStartWait  = 15 * time.Second
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
	handlers.SetLocalAPIStartupState(handlers.LocalAPIStartupInitializing, "local_api_starting")

	errCh := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("local api panic: %v", r)
			}
		}()
		if err := l.server.Start(); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-l.server.Ready():
		handlers.SetLocalAPIStartupState(handlers.LocalAPIStartupListening, "")
		logging.LogInfo(localAPIModuleName, "Local api listeners are ready")
		go func() {
			if err := <-errCh; err != nil {
				handlers.SetLocalAPIStartupState(handlers.LocalAPIStartupFailed, err.Error())
				logging.LogError(localAPIModuleName, "Local api server exited with error", err)
			}
		}()
		return nil
	case err := <-errCh:
		if err == nil {
			err = fmt.Errorf("local api server exited before signaling readiness")
		}
		handlers.SetLocalAPIStartupState(handlers.LocalAPIStartupFailed, err.Error())
		logging.LogError(localAPIModuleName, "Failed to start local api server", err)
		return err
	case <-time.After(localAPIStartWait):
		err := fmt.Errorf("local api listener readiness timed out after %s", localAPIStartWait)
		handlers.SetLocalAPIStartupState(handlers.LocalAPIStartupFailed, err.Error())
		logging.LogError(localAPIModuleName, "Failed to start local api server", err)
		return err
	}
}

// Stop stops the Local API server
func (l *LocalAPI) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logging.LogInfo(localAPIModuleName, "Stopping local api server")

	if l.server != nil {
		handlers.SetLocalAPIStartupState(handlers.LocalAPIStartupInitializing, "local_api_stopping")
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

// GetName returns the module name
func (l *LocalAPI) GetName() string {
	return localAPIModuleName
}

// GetModuleIndex returns the module index
func (l *LocalAPI) GetModuleIndex() int {
	return utils.LocalAPI
}
