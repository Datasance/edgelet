package fieldagent

import (
	"sync"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/version"
)

const (
	statusDeprovision401MaxAttempts = 5
	statusDeprovision401MinWindow   = 60 * time.Second
)

type statusAuthFailureState struct {
	mu             sync.Mutex
	count          int
	firstFailureAt time.Time
}

func (fa *FieldAgent) recordStatusAuthFailure(now time.Time) {
	fa.statusAuthFailure.mu.Lock()
	defer fa.statusAuthFailure.mu.Unlock()
	if fa.statusAuthFailure.count == 0 {
		fa.statusAuthFailure.firstFailureAt = now
	}
	fa.statusAuthFailure.count++
}

func (fa *FieldAgent) resetStatusAuthFailure() {
	fa.statusAuthFailure.mu.Lock()
	defer fa.statusAuthFailure.mu.Unlock()
	fa.statusAuthFailure.count = 0
	fa.statusAuthFailure.firstFailureAt = time.Time{}
}

func (fa *FieldAgent) shouldDeprovisionForStatusAuth(now time.Time) bool {
	fa.statusAuthFailure.mu.Lock()
	defer fa.statusAuthFailure.mu.Unlock()
	if fa.statusAuthFailure.count < statusDeprovision401MaxAttempts {
		return false
	}
	if fa.statusAuthFailure.firstFailureAt.IsZero() {
		return false
	}
	return now.Sub(fa.statusAuthFailure.firstFailureAt) >= statusDeprovision401MinWindow
}

func (fa *FieldAgent) statusAuthFailureCount() int {
	fa.statusAuthFailure.mu.Lock()
	defer fa.statusAuthFailure.mu.Unlock()
	return fa.statusAuthFailure.count
}

func (fa *FieldAgent) shouldSuppressStatusAuthDeprovision() bool {
	if pending, _ := version.ReadOTAReprovisionPending(); pending != nil {
		return true
	}
	return fa.isProvisionInFlight()
}

func (fa *FieldAgent) setProvisionInFlight(inFlight bool) {
	fa.provisionInFlight.Store(inFlight)
}

func (fa *FieldAgent) isProvisionInFlight() bool {
	return fa.provisionInFlight.Load()
}

func isStatusAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	return IsNonRetryableAgentAuthError(err) ||
		IsLegacyControllerAuthError(err) ||
		isUnauthorizedError(err)
}

func (fa *FieldAgent) statusAuthNow() time.Time {
	if fa.statusAuthNowFn != nil {
		return fa.statusAuthNowFn()
	}
	return time.Now()
}

func (fa *FieldAgent) invokeDeprovision(clearCredentials bool) error {
	if fa.deprovisionFn != nil {
		return fa.deprovisionFn(clearCredentials)
	}
	return fa.Deprovision(clearCredentials)
}
