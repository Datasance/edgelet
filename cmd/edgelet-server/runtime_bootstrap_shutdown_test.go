//go:build linux && cgo

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/constants"
	"github.com/eclipse-iofog/edgelet/pkg/containerd"
)

type fakeBootstrapStopService struct {
	stopCalled int
}

func (f *fakeBootstrapStopService) Stop() {
	f.stopCalled++
}

func TestRunDataPlaneDrainBeforeStop_InvokesDrainBeforeStop(t *testing.T) {
	t.Setenv("EDGELET_RUNTIME_SPLIT", "1")

	drainCalled := false
	stopSvc := &fakeBootstrapStopService{}
	deps := runtimeBootstrapShutdownDeps{
		shouldDrain: func() bool { return true },
		drain: func(drainSec int) dataPlaneDrainOutcome {
			if drainSec != 90 {
				t.Fatalf("expected drainSec=90, got %d", drainSec)
			}
			drainCalled = true
			return dataPlaneDrainOutcome{complete: true}
		},
	}

	runDataPlaneDrainBeforeStop(90, deps)
	stopSvc.Stop()

	if !drainCalled {
		t.Fatal("expected drain to be invoked on split runtime bootstrap stop")
	}
}

func TestRunDataPlaneDrainBeforeStop_MonolithicNoOp(t *testing.T) {
	t.Setenv("EDGELET_RUNTIME_SPLIT", "0")

	drainCalled := false
	deps := runtimeBootstrapShutdownDeps{
		shouldDrain: shouldDrainOnDataPlaneSIGTERM,
		drain: func(int) dataPlaneDrainOutcome {
			drainCalled = true
			return dataPlaneDrainOutcome{complete: true}
		},
	}

	runDataPlaneDrainBeforeStop(90, deps)
	if drainCalled {
		t.Fatal("expected drain to be skipped when runtime split is disabled")
	}
}

func TestRunDataPlaneDrainBeforeStop_DegradedProceed(t *testing.T) {
	deps := runtimeBootstrapShutdownDeps{
		shouldDrain: func() bool { return true },
		drain: func(int) dataPlaneDrainOutcome {
			return dataPlaneDrainOutcome{degraded: true}
		},
	}

	outcome := runDataPlaneDrainBeforeStop(45, deps)
	if outcome.complete || outcome.timedOut {
		t.Fatalf("expected degraded outcome, got %+v", outcome)
	}
}

func TestShouldDrainOnDataPlaneSIGTERM_SplitEnv(t *testing.T) {
	t.Setenv("EDGELET_RUNTIME_SPLIT", "1")
	if !shouldDrainOnDataPlaneSIGTERM() {
		t.Fatal("expected drain when EDGELET_RUNTIME_SPLIT=1")
	}
}

func TestShouldDrainOnDataPlaneSIGTERM_MonolithicEnv(t *testing.T) {
	t.Setenv("EDGELET_RUNTIME_SPLIT", "0")
	if shouldDrainOnDataPlaneSIGTERM() {
		t.Fatal("expected no drain when EDGELET_RUNTIME_SPLIT=0")
	}
}

func TestWaitForEdgeletAPISocket_NotReadyWithinBudget(t *testing.T) {
	// No production socket in unit-test environment; short budget should return false.
	if waitForEdgeletAPISocket(50 * time.Millisecond) {
		t.Fatal("expected waitForEdgeletAPISocket to time out without a listener")
	}
}

func TestIsRetryableRuntimeDrainError(t *testing.T) {
	cases := []struct {
		err    error
		stderr string
		want   bool
	}{
		{errors.New("dial unix /run/edgelet/edgelet.sock: connect: connection refused"), "", true},
		{errors.New("exit status 1"), "runtime engine is not ready", true},
		{errors.New("exit status 1"), "local_api_starting", true},
		{errors.New("exit status 1"), "local api is starting", true},
		{errors.New("exit status 1"), "failed to send Edgelet API request: Post \"http://unix/v1/runtime/drain\": EOF", true},
		{errors.New("exit status 1"), "drain timed out", false},
		{errors.New("exit status 1"), "permission denied", false},
	}
	for _, tc := range cases {
		if got := isRetryableRuntimeDrainError(tc.err, []byte(tc.stderr)); got != tc.want {
			t.Fatalf("isRetryableRuntimeDrainError(%q, %q) = %v, want %v", tc.err, tc.stderr, got, tc.want)
		}
	}
}

func TestExecRuntimeDrainCLI_RetriesBeforeSuccess(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	callFile := filepath.Join(t.TempDir(), "calls")
	scriptPath := filepath.Join(t.TempDir(), "edgelet")
	script := fmt.Sprintf(`#!/bin/sh
n=0
if [ -f %q ]; then
  read n < %q
fi
n=$((n+1))
echo "$n" > %q
if [ "$n" -lt 3 ]; then
  echo "connection refused" >&2
  exit 1
fi
exit 0
`, callFile, callFile, callFile)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake edgelet script: %v", err)
	}

	os.Args = []string{scriptPath, "runtime-bootstrap"}
	bootstrapTestWaitForAPISocket = func(time.Duration) bool { return true }
	t.Cleanup(func() { bootstrapTestWaitForAPISocket = nil })

	outcome := execRuntimeDrainCLI(1)
	if !outcome.complete || outcome.degraded || outcome.timedOut {
		t.Fatalf("expected complete outcome after retries, got %+v", outcome)
	}
	data, err := os.ReadFile(callFile)
	if err != nil {
		t.Fatalf("read call count: %v", err)
	}
	if string(data) != "3\n" {
		t.Fatalf("expected 3 drain CLI attempts, got %q", string(data))
	}
}

func TestExecRuntimeDrainCLI_RetriesOnEOF(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	callFile := filepath.Join(t.TempDir(), "calls")
	scriptPath := filepath.Join(t.TempDir(), "edgelet")
	script := fmt.Sprintf(`#!/bin/sh
n=0
if [ -f %q ]; then
  read n < %q
fi
n=$((n+1))
echo "$n" > %q
if [ "$n" -eq 1 ]; then
  echo '✘ failed to send Edgelet API request: Post "http://unix/v1/runtime/drain": EOF' >&2
  exit 1
fi
exit 0
`, callFile, callFile, callFile)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake edgelet script: %v", err)
	}

	os.Args = []string{scriptPath, "runtime-bootstrap"}
	bootstrapTestWaitForAPISocket = func(time.Duration) bool { return true }
	t.Cleanup(func() { bootstrapTestWaitForAPISocket = nil })

	outcome := execRuntimeDrainCLI(90)
	if !outcome.complete || outcome.degraded || outcome.timedOut {
		t.Fatalf("expected complete outcome after EOF retry, got %+v", outcome)
	}
}

func TestEdgeletOperatorBinary_UsesArgv0(t *testing.T) {
	orig := os.Args
	t.Cleanup(func() { os.Args = orig })
	os.Args = []string{"/usr/local/bin/edgelet", "runtime-bootstrap"}

	got, err := edgeletOperatorBinary()
	if err != nil {
		t.Fatalf("edgeletOperatorBinary: %v", err)
	}
	if got != "/usr/local/bin/edgelet" {
		t.Fatalf("expected argv0 binary, got %q", got)
	}
}

func TestStopEmbeddedContainerdDataPlane_DrainBeforeStopSinglePrimaryReap(t *testing.T) {
	var steps []string
	primaryReapCalls := 0
	svc := &fakeBootstrapStopService{}

	deps := defaultRuntimeBootstrapStopDeps()
	deps.shutdown.drain = func(drainSec int) dataPlaneDrainOutcome {
		steps = append(steps, fmt.Sprintf("drain:%d", drainSec))
		return dataPlaneDrainOutcome{complete: true}
	}
	deps.reapManagedShimsUntilClear = func(_ string, budget time.Duration) error {
		primaryReapCalls++
		steps = append(steps, fmt.Sprintf("reap:%d", primaryReapCalls))
		if budget <= 0 {
			t.Fatalf("expected primary reap budget > 0, got %v", budget)
		}
		return nil
	}
	deps.remainingStopBudget = func(_ time.Time, totalBudget time.Duration) time.Duration {
		return totalBudget
	}
	deps.setShimReapRemainingBudget = func(_ time.Duration) {}
	deps.resetShimReapRemainingBudget = func() {}

	stopEmbeddedContainerdDataPlane(constants.EdgeletContainerdSocket, 90, svc, deps)

	if svc.stopCalled != 1 {
		t.Fatalf("expected svc.Stop once, got %d", svc.stopCalled)
	}
	if primaryReapCalls != 1 {
		t.Fatalf("expected single primary reap before stop, got %d", primaryReapCalls)
	}
	if len(steps) != 2 || steps[0] != "drain:90" || steps[1] != "reap:1" {
		t.Fatalf("unexpected stop pipeline order: %v", steps)
	}
}

func TestStopEmbeddedContainerdDataPlane_VerifyReapWhenPrimaryIncomplete(t *testing.T) {
	reapCalls := 0
	svc := &fakeBootstrapStopService{}

	deps := defaultRuntimeBootstrapStopDeps()
	deps.shutdown.drain = func(int) dataPlaneDrainOutcome {
		return dataPlaneDrainOutcome{complete: true}
	}
	deps.reapManagedShimsUntilClear = func(_ string, budget time.Duration) error {
		reapCalls++
		if reapCalls == 1 {
			if budget <= 0 {
				t.Fatalf("expected primary reap budget > 0, got %v", budget)
			}
			return errors.New("managed runtime processes still running after reap attempts")
		}
		if budget <= 0 || budget > containerd.DefaultPostStopShimVerifyCap {
			t.Fatalf("expected verify reap budget in (0, %v], got %v", containerd.DefaultPostStopShimVerifyCap, budget)
		}
		return nil
	}
	deps.remainingStopBudget = func(_ time.Time, totalBudget time.Duration) time.Duration {
		return totalBudget
	}
	deps.setShimReapRemainingBudget = func(_ time.Duration) {}
	deps.resetShimReapRemainingBudget = func() {}

	stopEmbeddedContainerdDataPlane(constants.EdgeletContainerdSocket, 45, svc, deps)

	if svc.stopCalled != 1 {
		t.Fatalf("expected svc.Stop once, got %d", svc.stopCalled)
	}
	if reapCalls != 2 {
		t.Fatalf("expected primary + verify reap, got %d", reapCalls)
	}
}
