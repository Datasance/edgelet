package edgeletapi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/datasance/edgelet/internal/edgeletapi/handlers"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	localAPIModuleName = "Edgelet API"
	defaultPort        = 54321
	localAPIStartWait  = 15 * time.Second
)

// EdgeletAPI is the main Edgelet API module
type EdgeletAPI struct {
	server *Server
	mu     sync.RWMutex
}

var (
	instance *EdgeletAPI
	once     sync.Once
)

// GetInstance returns the singleton EdgeletAPI instance
func GetInstance() *EdgeletAPI {
	once.Do(func() {
		instance = &EdgeletAPI{}
	})
	return instance
}

// Start starts the Edgelet API server
func (l *EdgeletAPI) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logging.LogInfo(localAPIModuleName, "Start local api server")

	// Create and configure server
	l.server = NewServer(defaultPort)
	handlers.SetEdgeletAPIStartupState(handlers.EdgeletAPIStartupInitializing, "local_api_starting")

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
		handlers.SetEdgeletAPIStartupState(handlers.EdgeletAPIStartupListening, "")
		logging.LogInfo(localAPIModuleName, "Local api listeners are ready")
		go func() {
			if err := <-errCh; err != nil {
				handlers.SetEdgeletAPIStartupState(handlers.EdgeletAPIStartupFailed, err.Error())
				logging.LogError(localAPIModuleName, "Local api server exited with error", err)
			}
		}()
		return nil
	case err := <-errCh:
		if err == nil {
			err = errors.New("local api server exited before signaling readiness")
		}
		handlers.SetEdgeletAPIStartupState(handlers.EdgeletAPIStartupFailed, err.Error())
		logging.LogError(localAPIModuleName, "Failed to start local api server", err)
		return err
	case <-time.After(localAPIStartWait):
		err := fmt.Errorf("local api listener readiness timed out after %s", localAPIStartWait)
		handlers.SetEdgeletAPIStartupState(handlers.EdgeletAPIStartupFailed, err.Error())
		logging.LogError(localAPIModuleName, "Failed to start local api server", err)
		return err
	}
}

// Stop stops the Edgelet API server
func (l *EdgeletAPI) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logging.LogInfo(localAPIModuleName, "Stopping local api server")

	if l.server != nil {
		handlers.SetEdgeletAPIStartupState(handlers.EdgeletAPIStartupInitializing, "local_api_stopping")
		return l.server.Shutdown(context.Background())
	}

	return nil
}

// Update is called when configuration changes
func (l *EdgeletAPI) Update() {
	logging.LogDebug(localAPIModuleName, "Start the real-time control signal when the configuration updated")
	// This will be implemented to trigger control signals
	// For now, just log
	logging.LogDebug(localAPIModuleName, "Finish the real-time control signal when the configuration updated")
}

// GetName returns the module name
func (l *EdgeletAPI) GetName() string {
	return localAPIModuleName
}

// GetModuleIndex returns the module index
func (l *EdgeletAPI) GetModuleIndex() int {
	return utils.EdgeletAPI
}
