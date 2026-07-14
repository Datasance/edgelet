package processmanager

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/runtimestate"
)

func TestQuiesceBlocksReconcileGate(t *testing.T) {
	t.Cleanup(func() { SetQuiesced(false) })
	SetQuiesced(false)
	if IsQuiesced() {
		t.Fatal("expected not quiesced")
	}
	SetQuiesced(true)
	if !IsQuiesced() {
		t.Fatal("expected quiesced")
	}
	SetQuiesced(false)
}

func TestBeginQuiesceForDataPlaneDrain_HoldsUntilResume(t *testing.T) {
	t.Cleanup(func() { SetQuiesced(false) })
	BeginQuiesceForDataPlaneDrain()
	if !IsQuiesced() || !IsQuiescedForDataPlaneDrain() {
		t.Fatal("expected data-plane drain quiesce hold")
	}
	SetQuiesced(false)
	if IsQuiescedForDataPlaneDrain() {
		t.Fatal("expected SetQuiesced(false) to clear data-plane drain hold")
	}
}

func TestTryResumeReconcileAfterDataPlaneEngineReady_SetsEngineReady(t *testing.T) {
	runtimestate.ResetForTests()
	t.Cleanup(runtimestate.ResetForTests)
	t.Cleanup(func() { SetQuiesced(false) })

	BeginQuiesceForDataPlaneDrain()
	runtimestate.GetState().SetEngineReady(false)
	TryResumeReconcileAfterDataPlaneEngineReady()
	if !runtimestate.GetState().EngineReady() {
		t.Fatal("expected engine ready after resume")
	}
}
