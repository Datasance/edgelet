package edgeresources

import (
	"fmt"
	"sync"

	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	moduleName = "EdgeResource Manager"
)

// Manager manages edge resources with thread-safe access
type Manager struct {
	latestEdgeResources  []*models.EdgeResource
	currentEdgeResources []*models.EdgeResource
	mu                   sync.RWMutex
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton EdgeResourceManager instance
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			latestEdgeResources:  make([]*models.EdgeResource, 0),
			currentEdgeResources: make([]*models.EdgeResource, 0),
		}
	})
	return instance
}

// GetLatestEdgeResources returns an unmodifiable copy of the latest edge resources
func (erm *Manager) GetLatestEdgeResources() []*models.EdgeResource {
	erm.mu.RLock()
	defer erm.mu.RUnlock()

	result := make([]*models.EdgeResource, len(erm.latestEdgeResources))
	copy(result, erm.latestEdgeResources)
	return result
}

// SetLatestEdgeResources sets the latest edge resources
func (erm *Manager) SetLatestEdgeResources(edgeResources []*models.EdgeResource) {
	erm.mu.Lock()
	defer erm.mu.Unlock()

	erm.latestEdgeResources = make([]*models.EdgeResource, len(edgeResources))
	copy(erm.latestEdgeResources, edgeResources)
}

// GetCurrentEdgeResources returns an unmodifiable copy of the current edge resources
func (erm *Manager) GetCurrentEdgeResources() []*models.EdgeResource {
	erm.mu.RLock()
	defer erm.mu.RUnlock()

	result := make([]*models.EdgeResource, len(erm.currentEdgeResources))
	copy(result, erm.currentEdgeResources)
	return result
}

// SetCurrentEdgeResources sets the current edge resources
func (erm *Manager) SetCurrentEdgeResources(edgeResources []*models.EdgeResource) {
	erm.mu.Lock()
	defer erm.mu.Unlock()

	erm.currentEdgeResources = make([]*models.EdgeResource, len(edgeResources))
	copy(erm.currentEdgeResources, edgeResources)
}

// Clear clears all edge resources
func (erm *Manager) Clear() {
	erm.mu.Lock()
	defer erm.mu.Unlock()

	latestSize := len(erm.latestEdgeResources)
	currentSize := len(erm.currentEdgeResources)
	logging.LogDebug(moduleName, fmt.Sprintf("Start clearing EdgeResources, size of latestEdgeResources and currentEdgeResources is respectively: %d, %d", latestSize, currentSize))

	erm.latestEdgeResources = make([]*models.EdgeResource, 0)
	erm.currentEdgeResources = make([]*models.EdgeResource, 0)

	logging.LogDebug(moduleName, "Finished clearing EdgeResources")
}
