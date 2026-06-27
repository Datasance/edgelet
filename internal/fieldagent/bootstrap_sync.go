package fieldagent

import (
	"fmt"
	"runtime/debug"

	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

// bootstrapControllerSync loads cached controller state asynchronously, then refreshes
// from the controller when reachable. It must not block supervisor startup.
func (fa *FieldAgent) bootstrapControllerSync() {
	defer fa.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(moduleName, "Panic in bootstrapControllerSync", fmt.Errorf("%v\n%s", r, debug.Stack()))
		}
	}()

	if fa.NotProvisioned() {
		logging.LogDebug(moduleName, "Skipping controller bootstrap: agent not provisioned")
		return
	}

	logging.LogInfo(moduleName, "Bootstrapping controller data from local cache")
	fa.loadInitialControllerData(false)
	fa.setBootstrapCacheLoaded(true)
	fa.notifyProcessManagerAfterBootstrap()

	fa.maybeReprovisionAfterOTA()

	if fa.ping() {
		logging.LogInfo(moduleName, "Controller reachable at boot; reconciling controller data")
		if err := fa.controllerReconcile(); err != nil {
			logging.LogWarn(moduleName, fmt.Sprintf("Controller reconcile during boot sync failed: %v", err))
		}
	} else {
		logging.LogWarn(moduleName, "Controller unreachable at boot; continuing with cached data")
	}
	fa.notifyProcessManagerAfterBootstrap()
}

func (fa *FieldAgent) setBootstrapCacheLoaded(v bool) {
	fa.bootstrapMu.Lock()
	fa.bootstrapCacheLoaded = v
	fa.bootstrapMu.Unlock()
}

func (fa *FieldAgent) isBootstrapCacheLoaded() bool {
	fa.bootstrapMu.RLock()
	defer fa.bootstrapMu.RUnlock()
	return fa.bootstrapCacheLoaded
}

// OnProcessManagerReady notifies the process manager after cache bootstrap when PM
// is wired later in supervisor startup.
func (fa *FieldAgent) OnProcessManagerReady() {
	if fa.isBootstrapCacheLoaded() {
		fa.notifyProcessManagerAfterBootstrap()
	}
}

func (fa *FieldAgent) notifyProcessManagerAfterBootstrap() {
	fa.mu.RLock()
	pm := fa.processManager
	fa.mu.RUnlock()
	if pm == nil {
		return
	}
	logging.LogDebug(moduleName, "Notifying ProcessManager after controller bootstrap")
	pm.Update()
}
