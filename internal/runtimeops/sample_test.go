package runtimeops

import (
	"context"
	"os"
	"testing"
)

func TestEmit_DebugSamplingSkipsLogButNotTestSink(t *testing.T) {
	var sinkCount int
	SetTestSink(func(RuntimeEvent) { sinkCount++ })
	t.Cleanup(func() {
		SetTestSink(nil)
		ResetDebugSampleRateForTest()
	})

	SetDebugSampleRateForTest(0)
	for i := 0; i < 20; i++ {
		Emit(context.Background(), RuntimeEvent{
			Event:   EventEngineContainerStart,
			Level:   LevelDebug,
			Message: "debug",
		})
	}
	if sinkCount != 20 {
		t.Fatalf("test sink count=%d want 20", sinkCount)
	}
}

func TestDebugLogEnabled_FromEnv(t *testing.T) {
	t.Setenv("LOG_DEBUG_SAMPLE_RATE", "0")
	ResetDebugSampleRateForTest()
	if debugLogEnabled() {
		t.Fatal("expected rate 0 to disable debug logs")
	}
	_ = os.Getenv("LOG_DEBUG_SAMPLE_RATE")
}
