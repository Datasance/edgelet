package processmanager

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/datasance/edgelet/internal/models"
)

// localLaunchInFlightStaleTimeout allows reconcile to retry a launch when a prior
// attempt (CLI apply or reconcile) appears stuck without observing the generation.
const localLaunchInFlightStaleTimeout = 30 * time.Minute

// localDeploymentLaunchInFlight reports whether a local deployment record indicates
// an active launch owned by ApplyLocalManifest (starting, generation not yet observed).
func localDeploymentLaunchInFlight(item *models.LocalDeployedMicroservice, now int64) bool {
	if item == nil {
		return false
	}
	if strings.ToLower(strings.TrimSpace(item.RuntimeState)) != "starting" {
		return false
	}
	if item.Generation <= item.ObservedGeneration {
		return false
	}
	startedAt := item.LastStartAttemptAt
	if startedAt <= 0 {
		startedAt = item.LastTransitionAt
	}
	if startedAt > 0 && now-startedAt > int64(localLaunchInFlightStaleTimeout.Seconds()) {
		return false
	}
	return true
}

func (pm *ProcessManager) withLocalLaunchLock(microserviceUUID string, fn func() (string, error)) (string, error) {
	uuid := strings.TrimSpace(microserviceUUID)
	if uuid == "" {
		return fn()
	}
	v, _ := pm.localLaunchLocks.LoadOrStore(uuid, &sync.Mutex{})
	mu, ok := v.(*sync.Mutex)
	if !ok {
		return "", errors.New("local launch lock has unexpected type")
	}
	mu.Lock()
	defer mu.Unlock()
	return fn()
}
