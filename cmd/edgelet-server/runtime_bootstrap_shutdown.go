//go:build linux && cgo

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/internal/utils"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/pkg/containerd"
)

const (
	edgeletAPISocketWaitBudget = 45 * time.Second
	edgeletAPISocketPoll       = 500 * time.Millisecond
)

// bootstrapTestWaitForAPISocket, when set, overrides waitForEdgeletAPISocket (tests only).
var bootstrapTestWaitForAPISocket func(time.Duration) bool

type dataPlaneDrainOutcome struct {
	complete bool
	timedOut bool
	degraded bool
}

type runtimeBootstrapShutdownDeps struct {
	shouldDrain func() bool
	drain       func(drainSec int) dataPlaneDrainOutcome
}

func defaultRuntimeBootstrapShutdownDeps() runtimeBootstrapShutdownDeps {
	return runtimeBootstrapShutdownDeps{
		shouldDrain: shouldDrainOnDataPlaneSIGTERM,
		drain:       execRuntimeDrainCLI,
	}
}

// shouldDrainOnDataPlaneSIGTERM reports whether runtime-bootstrap should invoke
// coordinated MS drain before stopping embedded containerd (runtime split only).
func shouldDrainOnDataPlaneSIGTERM() bool {
	if os.Getenv("EDGELET_RUNTIME_SPLIT") == "0" {
		return false
	}
	if logging.RuntimeSplitFromEnv() {
		return true
	}
	// runtime-bootstrap runs on edgelet-containerd.service (split data plane).
	return true
}

func runDataPlaneDrainBeforeStop(drainSec int, deps runtimeBootstrapShutdownDeps) dataPlaneDrainOutcome {
	if deps.shouldDrain != nil && !deps.shouldDrain() {
		return dataPlaneDrainOutcome{}
	}
	if deps.drain == nil {
		deps.drain = execRuntimeDrainCLI
	}

	logging.LogInfo("RUNTIME_BOOTSTRAP", "drain_started")
	outcome := deps.drain(drainSec)
	switch {
	case outcome.complete:
		logging.LogInfo("RUNTIME_BOOTSTRAP", "drain_complete")
	case outcome.timedOut:
		logging.LogWarn("RUNTIME_BOOTSTRAP", "drain_timeout")
	default:
		logging.LogWarn("RUNTIME_BOOTSTRAP", "drain_degraded")
	}
	return outcome
}

func edgeletAPISocketCandidates() []string {
	return []string{
		filepath.Join(utils.VarRun, "edgelet.sock"),
		filepath.Join(constants.EdgeletRunDir, "edgelet.sock"),
	}
}

func waitForEdgeletAPISocket(budget time.Duration) bool {
	if bootstrapTestWaitForAPISocket != nil {
		return bootstrapTestWaitForAPISocket(budget)
	}
	if budget <= 0 {
		budget = edgeletAPISocketWaitBudget
	}
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		for _, path := range edgeletAPISocketCandidates() {
			if _, err := os.Stat(path); err == nil {
				return true
			}
		}
		time.Sleep(edgeletAPISocketPoll)
	}
	return false
}

func isRetryableRuntimeDrainError(err error, stderr []byte) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error() + " " + string(stderr))
	return strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "failed to read response") ||
		strings.Contains(msg, "runtime engine is not ready") ||
		strings.Contains(msg, "local_api_starting") ||
		strings.Contains(msg, "local api is starting")
}

func execRuntimeDrainCLI(drainSec int) dataPlaneDrainOutcome {
	bin, err := edgeletOperatorBinary()
	if err != nil {
		return dataPlaneDrainOutcome{degraded: true}
	}

	// Allow CLI/API overhead beyond the drain budget (BR-24-E6 total stop ≤120s).
	outerBudget := time.Duration(drainSec+15) * time.Second
	deadline := time.Now().Add(edgeletAPISocketWaitBudget)

	for time.Now().Before(deadline) {
		if !waitForEdgeletAPISocket(edgeletAPISocketPoll) {
			time.Sleep(edgeletAPISocketPoll)
			continue
		}

		var stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), outerBudget)
		cmd := exec.CommandContext(ctx, bin, "runtime", "drain", "--timeout", fmt.Sprintf("%d", drainSec)) // #nosec G204 -- operator binary from current edgelet argv/LookPath; fixed subcommand args
		cmd.Stdout = os.Stderr
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

		runErr := cmd.Run()
		cancel()
		if runErr == nil {
			return dataPlaneDrainOutcome{complete: true}
		}

		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			switch exitErr.ExitCode() {
			case 1:
				stderrText := strings.ToLower(stderr.String())
				if strings.Contains(stderrText, "timed out") {
					return dataPlaneDrainOutcome{timedOut: true}
				}
				if isRetryableRuntimeDrainError(runErr, stderr.Bytes()) {
					time.Sleep(edgeletAPISocketPoll)
					continue
				}
				return dataPlaneDrainOutcome{degraded: true}
			default:
				if isRetryableRuntimeDrainError(runErr, stderr.Bytes()) {
					time.Sleep(edgeletAPISocketPoll)
					continue
				}
				return dataPlaneDrainOutcome{degraded: true}
			}
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return dataPlaneDrainOutcome{timedOut: true}
		}
		if isRetryableRuntimeDrainError(runErr, stderr.Bytes()) {
			time.Sleep(edgeletAPISocketPoll)
			continue
		}
		return dataPlaneDrainOutcome{degraded: true}
	}

	logging.LogWarn("RUNTIME_BOOTSTRAP", "edgelet API drain retries exhausted before data-plane stop")
	return dataPlaneDrainOutcome{degraded: true}
}

func edgeletOperatorBinary() (string, error) {
	if len(os.Args) > 0 {
		if path := os.Args[0]; path != "" {
			if resolved, err := exec.LookPath(path); err == nil {
				return resolved, nil
			}
			return path, nil
		}
	}
	return exec.LookPath("edgelet")
}

type runtimeBootstrapStopper interface {
	Stop()
}

type runtimeBootstrapStopDeps struct {
	shutdown                     runtimeBootstrapShutdownDeps
	reapManagedShimsUntilClear   func(socketPath string, remainingBudget time.Duration) error
	remainingStopBudget          func(stopStarted time.Time, totalBudget time.Duration) time.Duration
	setShimReapRemainingBudget   func(remaining time.Duration)
	resetShimReapRemainingBudget func()
	dataPlaneStopBudget          time.Duration
	postStopShimVerifyCap        time.Duration
	stopStarted                  func() time.Time
}

func defaultRuntimeBootstrapStopDeps() runtimeBootstrapStopDeps {
	return runtimeBootstrapStopDeps{
		shutdown:                     defaultRuntimeBootstrapShutdownDeps(),
		reapManagedShimsUntilClear:   containerd.ReapManagedShimsUntilClear,
		remainingStopBudget:          containerd.RemainingStopBudget,
		setShimReapRemainingBudget:   containerd.SetShimReapRemainingBudget,
		resetShimReapRemainingBudget: containerd.ResetShimReapRemainingBudget,
		dataPlaneStopBudget:          containerd.DefaultDataPlaneStopBudget,
		postStopShimVerifyCap:        containerd.DefaultPostStopShimVerifyCap,
		stopStarted:                  time.Now,
	}
}

func (d runtimeBootstrapStopDeps) withDefaults() runtimeBootstrapStopDeps {
	defaults := defaultRuntimeBootstrapStopDeps()
	if d.shutdown.shouldDrain == nil {
		d.shutdown.shouldDrain = defaults.shutdown.shouldDrain
	}
	if d.shutdown.drain == nil {
		d.shutdown.drain = defaults.shutdown.drain
	}
	if d.reapManagedShimsUntilClear == nil {
		d.reapManagedShimsUntilClear = defaults.reapManagedShimsUntilClear
	}
	if d.remainingStopBudget == nil {
		d.remainingStopBudget = defaults.remainingStopBudget
	}
	if d.setShimReapRemainingBudget == nil {
		d.setShimReapRemainingBudget = defaults.setShimReapRemainingBudget
	}
	if d.resetShimReapRemainingBudget == nil {
		d.resetShimReapRemainingBudget = defaults.resetShimReapRemainingBudget
	}
	if d.dataPlaneStopBudget <= 0 {
		d.dataPlaneStopBudget = defaults.dataPlaneStopBudget
	}
	if d.postStopShimVerifyCap <= 0 {
		d.postStopShimVerifyCap = defaults.postStopShimVerifyCap
	}
	if d.stopStarted == nil {
		d.stopStarted = defaults.stopStarted
	}
	return d
}

func stopEmbeddedContainerdDataPlane(socketPath string, drainSec int, svc runtimeBootstrapStopper, deps runtimeBootstrapStopDeps) {
	deps = deps.withDefaults()
	stopStart := deps.stopStarted()
	totalBudget := deps.dataPlaneStopBudget

	runDataPlaneDrainBeforeStop(drainSec, deps.shutdown)

	primaryReapBudget := deps.remainingStopBudget(stopStart, totalBudget)
	var primaryReapErr error
	if primaryReapBudget <= 0 {
		logging.LogWarn("RUNTIME_BOOTSTRAP", "primary managed shim reap skipped: stop budget exhausted after drain")
		primaryReapErr = errors.New("stop budget exhausted after drain")
	} else if err := deps.reapManagedShimsUntilClear(socketPath, primaryReapBudget); err != nil {
		logging.LogWarn("RUNTIME_BOOTSTRAP", fmt.Sprintf("primary managed shim reap incomplete: %v", err))
		primaryReapErr = err
	}

	stopReapBudget := deps.remainingStopBudget(stopStart, totalBudget)
	deps.setShimReapRemainingBudget(stopReapBudget)
	defer deps.resetShimReapRemainingBudget()

	svc.Stop()

	if primaryReapErr == nil {
		return
	}

	verifyBudget := deps.remainingStopBudget(stopStart, totalBudget)
	if verifyBudget > deps.postStopShimVerifyCap {
		verifyBudget = deps.postStopShimVerifyCap
	}
	if verifyBudget <= 0 {
		return
	}
	if verifyErr := deps.reapManagedShimsUntilClear(socketPath, verifyBudget); verifyErr != nil {
		logging.LogWarn("RUNTIME_BOOTSTRAP", fmt.Sprintf("post-stop managed shim verify incomplete: %v", verifyErr))
	}
}
