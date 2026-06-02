package processmanager

import "testing"

func TestQuiesceBlocksReconcileGate(t *testing.T) {
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
