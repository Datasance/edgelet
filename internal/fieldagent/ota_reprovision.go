package fieldagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/internal/version"
)

var otaReprovisionRetry atomic.Bool

func (fa *FieldAgent) fetchControllerVersion() (map[string]any, error) {
	if fa.apiClient == nil {
		return nil, errors.New("api client not initialized")
	}
	ctx, cancel := context.WithTimeout(fa.ctx, 30*time.Second)
	defer cancel()
	return fa.apiClient.Request(ctx, "version", GET, nil, nil)
}

func (fa *FieldAgent) maybeReprovisionAfterOTA() {
	receipt, err := version.NewReleaseManager().ReadInstallReceipt()
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("OTA reprovision skipped: read install receipt failed: %v", err))
		return
	}
	if receipt == nil || !isOTAInstallMethod(receipt.InstallMethod) {
		return
	}

	pending, err := version.ReadOTAReprovisionPending()
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("OTA reprovision skipped: read pending failed: %v", err))
		return
	}
	if pending == nil {
		return
	}

	now := time.Now()
	if pending.IsExpired(now) {
		if fa.tryRefreshOTAReprovisionPending() {
			pending, err = version.ReadOTAReprovisionPending()
			if err != nil || pending == nil {
				logging.LogWarn(moduleName, "OTA reprovision refresh did not yield pending state")
				otaReprovisionRetry.Store(true)
				return
			}
		} else {
			logging.LogWarn(moduleName, "OTA reprovision key expired; keeping existing credentials and retrying on upgrade scan")
			_ = version.DeleteOTAReprovisionPending()
			otaReprovisionRetry.Store(true)
			return
		}
	}

	if pending.IsExpired(time.Now()) {
		logging.LogWarn(moduleName, "OTA reprovision key still expired after refresh; keeping existing credentials")
		_ = version.DeleteOTAReprovisionPending()
		otaReprovisionRetry.Store(true)
		return
	}

	if err := fa.Provision(pending.ProvisionKey); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("OTA reprovision failed: %v", err))
		otaReprovisionRetry.Store(true)
		return
	}

	if err := version.DeleteOTAReprovisionPending(); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("OTA reprovision succeeded but failed to delete pending file: %v", err))
	} else {
		logging.LogInfo(moduleName, "OTA reprovision completed; pending file removed")
	}
	otaReprovisionRetry.Store(false)

	if err := fa.postFogConfig(); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("postFogConfig after OTA reprovision failed: %v", err))
	}
}

func (fa *FieldAgent) tryRefreshOTAReprovisionPending() bool {
	raw, err := fa.fetchControllerVersion()
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("OTA reprovision version refresh failed: %v", err))
		return false
	}

	actionData, err := version.NormalizeVersionResponse(raw)
	if err != nil || actionData == nil {
		logging.LogWarn(moduleName, fmt.Sprintf("OTA reprovision version refresh normalize failed: %v", err))
		return false
	}

	pending, err := version.PendingFromAction(actionData)
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("OTA reprovision refresh missing valid key: %v", err))
		return false
	}
	if pending.IsExpired(time.Now()) {
		return false
	}

	if err := version.WriteOTAReprovisionPending(
		pending.ProvisionKey,
		pending.Command,
		pending.TargetVersion,
		pending.ExpirationTime,
	); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("OTA reprovision refresh write pending failed: %v", err))
		return false
	}
	logging.LogInfo(moduleName, "OTA reprovision key refreshed from controller")
	return true
}

func (fa *FieldAgent) retryOTAReprovisionIfNeeded() {
	if !otaReprovisionRetry.Load() {
		return
	}
	if fa.NotProvisioned() || !fa.IsControllerConnected(false) {
		return
	}

	pending, err := version.ReadOTAReprovisionPending()
	if err != nil || pending == nil {
		if pending == nil {
			if fa.tryRefreshOTAReprovisionPending() {
				fa.maybeReprovisionAfterOTA()
			}
		}
		return
	}

	fa.maybeReprovisionAfterOTA()
}

func isOTAInstallMethod(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "upgrade", "upgrade-airgap", "rollback":
		return true
	default:
		return false
	}
}
