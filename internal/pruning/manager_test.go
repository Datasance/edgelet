package pruning

import "testing"

func TestShouldRunImmediateFrequencyPrune(t *testing.T) {
	m := &Manager{}

	if m.shouldRunImmediateFrequencyPrune(0) {
		t.Fatalf("expected no immediate run for disabled frequency")
	}

	if !m.shouldRunImmediateFrequencyPrune(1) {
		t.Fatalf("expected immediate run when enabling frequency from 0 to 1")
	}
	m.setLastAppliedPruningFrequency(1)

	if m.shouldRunImmediateFrequencyPrune(1) {
		t.Fatalf("expected no immediate run when frequency does not change")
	}

	if !m.shouldRunImmediateFrequencyPrune(2) {
		t.Fatalf("expected immediate run when frequency changes from 1 to 2")
	}

	m.setLastAppliedPruningFrequency(2)
	if !m.shouldRunImmediateFrequencyPrune(1) {
		t.Fatalf("expected immediate run when frequency changes from 2 to 1")
	}
}
