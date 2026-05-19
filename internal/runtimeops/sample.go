package runtimeops

import (
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	debugSampleOnce sync.Once
	debugSampleRate float64 = 1.0
)

// debugLogEnabled returns whether a Debug-level runtimeops event should be written to the log backend.
// Test sinks and metrics hooks are unaffected. LOG_DEBUG_SAMPLE_RATE in [0,1] controls the fraction kept
// (unset or invalid → 1.0, no sampling).
func debugLogEnabled() bool {
	debugSampleOnce.Do(initDebugSampleRate)
	if debugSampleRate >= 1.0 {
		return true
	}
	if debugSampleRate <= 0 {
		return false
	}
	return rand.Float64() < debugSampleRate
}

func initDebugSampleRate() {
	raw := strings.TrimSpace(os.Getenv("LOG_DEBUG_SAMPLE_RATE"))
	if raw == "" {
		return
	}
	rate, err := strconv.ParseFloat(raw, 64)
	if err != nil || rate < 0 || rate > 1 {
		return
	}
	debugSampleRate = rate
}

// ResetDebugSampleRateForTest resets sampling state (tests only).
func ResetDebugSampleRateForTest() {
	debugSampleOnce = sync.Once{}
	debugSampleRate = 1.0
}

// SetDebugSampleRateForTest sets the sample rate without reading the environment (tests only).
func SetDebugSampleRateForTest(rate float64) {
	debugSampleOnce = sync.Once{}
	debugSampleRate = rate
}
