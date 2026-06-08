package processmanager

import (
	"sync"
	"time"
)

const (
	// IntervalInMinutes is the time window for checking stuck restarts
	IntervalInMinutes = 10
	// AbnormalNumberOfRestarts is the threshold for considering a microservice stuck
	AbnormalNumberOfRestarts = 10
)

// RestartStuckChecker tracks restart timestamps to detect stuck microservices
type RestartStuckChecker struct {
	restarts          map[string][]time.Time
	containerCreation map[string][]time.Time
	mu                sync.RWMutex
}

var (
	restartCheckerInstance *RestartStuckChecker
	restartCheckerOnce     sync.Once
)

// GetRestartStuckChecker returns the singleton RestartStuckChecker instance
func GetRestartStuckChecker() *RestartStuckChecker {
	restartCheckerOnce.Do(func() {
		restartCheckerInstance = &RestartStuckChecker{
			restarts:          make(map[string][]time.Time),
			containerCreation: make(map[string][]time.Time),
		}
	})
	return restartCheckerInstance
}

// IsStuck checks if a microservice is stuck in restart loop
// Returns true if the microservice has restarted 10+ times in the last 10 minutes
func (r *RestartStuckChecker) IsStuck(microserviceUUID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoffTime := now.Add(-IntervalInMinutes * time.Minute)

	// Get or create the restart timestamps list for this microservice
	datesOfRestart, exists := r.restarts[microserviceUUID]
	if !exists {
		datesOfRestart = make([]time.Time, 0)
	}

	// Remove timestamps older than the interval
	filtered := make([]time.Time, 0, len(datesOfRestart))
	for _, date := range datesOfRestart {
		if date.After(cutoffTime) {
			filtered = append(filtered, date)
		}
	}

	// Add current timestamp
	filtered = append(filtered, now)
	r.restarts[microserviceUUID] = filtered

	// Check if we've exceeded the threshold
	return len(filtered) >= AbnormalNumberOfRestarts
}

// IsStuckInContainerCreation checks if a microservice is stuck in container creation
// Returns true if the microservice has attempted container creation 10+ times in the last 10 minutes
func (r *RestartStuckChecker) IsStuckInContainerCreation(microserviceUUID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoffTime := now.Add(-IntervalInMinutes * time.Minute)

	// Get or create the creation timestamps list for this microservice
	datesOfCreation, exists := r.containerCreation[microserviceUUID]
	if !exists {
		datesOfCreation = make([]time.Time, 0)
	}

	// Remove timestamps older than the interval
	filtered := make([]time.Time, 0, len(datesOfCreation))
	for _, date := range datesOfCreation {
		if date.After(cutoffTime) {
			filtered = append(filtered, date)
		}
	}

	// Add current timestamp
	filtered = append(filtered, now)
	r.containerCreation[microserviceUUID] = filtered

	// Check if we've exceeded the threshold
	return len(filtered) >= AbnormalNumberOfRestarts
}

// Clear removes all tracking data for a microservice (useful for cleanup)
func (r *RestartStuckChecker) Clear(microserviceUUID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.restarts, microserviceUUID)
	delete(r.containerCreation, microserviceUUID)
}
