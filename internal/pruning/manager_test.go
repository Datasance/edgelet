package pruning

import (
	"testing"

	"github.com/eclipse-iofog/agent/internal/workloadmeta"
	"github.com/eclipse-iofog/agent/pkg/engine"
)

func TestIsManagedContainer_UsesCanonicalLabelsOnly(t *testing.T) {
	managed := engine.Container{
		Labels: map[string]string{
			workloadmeta.LabelAppManagedBy:    workloadmeta.ManagedByValue,
			workloadmeta.LabelMicroserviceUID: "ms-1",
		},
	}
	if !isManagedContainer(managed) {
		t.Fatal("expected canonical managed labels to be treated as managed")
	}

	nonCanonicalOnly := engine.Container{
		Labels: map[string]string{
			"example.com/pretend-service": "x",
			"example.com/pretend-node":    "y",
		},
	}
	if isManagedContainer(nonCanonicalOnly) {
		t.Fatal("containers without canonical managed-by + microservice uid must not be treated as managed")
	}
}

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
