package network

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
)

func newTestManager(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

func TestValidateNetworkInterfaceConfig(t *testing.T) {
	mgr := GetInstance()

	if err := mgr.ValidateNetworkInterfaceConfig("http://127.0.0.1:51121", "dynamic"); err != nil {
		t.Fatalf("expected dynamic interface config to be accepted, got error: %v", err)
	}

	if err := mgr.ValidateNetworkInterfaceConfig("http://127.0.0.1:51121", "iface-does-not-exist-98765"); err == nil {
		t.Fatal("expected unknown interface to be rejected")
	}
}

func TestStart_NoRecursionWhenNoIPv4(t *testing.T) {
	mgr := newTestManager(&config.Config{
		ControllerURL:    "http://127.0.0.1:51121",
		NetworkInterface: "dynamic",
	})
	mgr.testSyncRetrySpacing = time.Millisecond
	mgr.testAsyncBaseBackoff = time.Hour
	mgr.resolveNetworkInterfaceHook = func(_, _ string) (*NetworkInterfaceInfo, error) {
		return nil, errors.New("no suitable network interface found")
	}
	mgr.getAnyIPv4AddressHook = func() (string, error) {
		return "", errors.New("no suitable IPv4 address found")
	}

	done := make(chan struct{})
	go func() {
		if err := mgr.Start(); err != nil {
			t.Errorf("Start() = %v, want nil", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start() did not return within 3s (possible recursion or unbounded blocking)")
	}

	if ip := mgr.GetCurrentIPAddress(); ip != "" {
		t.Fatalf("expected empty IP in degraded mode, got %q", ip)
	}
	_ = mgr.Stop()
}

func TestStart_FallbackIPv4WhenControllerUnreachable(t *testing.T) {
	mgr := newTestManager(&config.Config{
		ControllerURL:    "http://127.0.0.1:51121",
		NetworkInterface: "dynamic",
	})
	mgr.resolveNetworkInterfaceHook = func(_, _ string) (*NetworkInterfaceInfo, error) {
		return nil, errors.New("controller unreachable")
	}
	mgr.getAnyIPv4AddressHook = func() (string, error) {
		return "192.168.1.42", nil
	}

	if err := mgr.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if ip := mgr.GetCurrentIPAddress(); ip != "192.168.1.42" {
		t.Fatalf("GetCurrentIPAddress() = %q, want 192.168.1.42", ip)
	}
	_ = mgr.Stop()
}

func TestStart_SyncAttemptsBounded(t *testing.T) {
	var attempts int32
	mgr := newTestManager(&config.Config{
		ControllerURL:    "http://127.0.0.1:51121",
		NetworkInterface: "dynamic",
	})
	mgr.testSyncRetrySpacing = time.Millisecond
	mgr.testAsyncBaseBackoff = time.Hour
	mgr.resolveNetworkInterfaceHook = func(_, _ string) (*NetworkInterfaceInfo, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("no interface")
	}
	mgr.getAnyIPv4AddressHook = func() (string, error) {
		return "", errors.New("no IPv4")
	}

	start := time.Now()
	if err := mgr.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	elapsed := time.Since(start)

	if got := atomic.LoadInt32(&attempts); got != networkSyncMaxAttempts {
		t.Fatalf("sync attempts = %d, want %d", got, networkSyncMaxAttempts)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("Start() took %v, want <= 3s", elapsed)
	}
	_ = mgr.Stop()
}

func TestPeriodicUpdate_NoGoroutineRespawn(t *testing.T) {
	mgr := newTestManager(&config.Config{
		ControllerURL:    "http://127.0.0.1:51121",
		NetworkInterface: "dynamic",
	})
	mgr.testPeriodicEmpty = 5 * time.Millisecond
	mgr.currentIPAddress = "10.0.0.1"
	mgr.resolveNetworkInterfaceHook = func(_, _ string) (*NetworkInterfaceInfo, error) {
		return nil, errors.New("forced update error")
	}

	go mgr.periodicUpdate()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&mgr.periodicUpdateActive) > 1 {
			t.Fatalf("periodicUpdate goroutine respawn detected: active=%d", atomic.LoadInt32(&mgr.periodicUpdateActive))
		}
		time.Sleep(2 * time.Millisecond)
	}

	_ = mgr.Stop()
	time.Sleep(20 * time.Millisecond)
	if active := atomic.LoadInt32(&mgr.periodicUpdateActive); active != 0 {
		t.Fatalf("periodicUpdate still active after Stop(): %d", active)
	}
}

func TestPeriodicInterval_EmptyVsSet(t *testing.T) {
	mgr := newTestManager(&config.Config{})

	if got := mgr.periodicInterval(); got != networkPeriodicIntervalEmpty {
		t.Fatalf("empty IP interval = %v, want %v", got, networkPeriodicIntervalEmpty)
	}

	mgr.mu.Lock()
	mgr.currentIPAddress = "10.0.0.5"
	mgr.mu.Unlock()

	if got := mgr.periodicInterval(); got != networkPeriodicIntervalSet {
		t.Fatalf("set IP interval = %v, want %v", got, networkPeriodicIntervalSet)
	}
}
