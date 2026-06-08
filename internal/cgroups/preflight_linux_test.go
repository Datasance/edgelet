//go:build linux && !cgo

package cgroups

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestDetectModeHost(t *testing.T) {
	mode, mount, _ := detectModeHost()
	if mode == ModeUnknown {
		t.Fatalf("expected detectable cgroup mode, got unknown (mount=%q)", mount)
	}
	if mount == "" {
		t.Fatal("expected non-empty unified mountpoint")
	}
}

func TestDetectNestedHostDockerenv(t *testing.T) {
	prevStat := statFn
	t.Cleanup(func() { statFn = prevStat })
	statFn = func(name string) (os.FileInfo, error) {
		if name == "/.dockerenv" {
			return fakeDirInfo{}, nil
		}
		return prevStat(name)
	}
	if !detectNestedHost() {
		t.Fatal("expected nested=true when /.dockerenv exists")
	}
}

func TestSelectDriverHostNestedPrefersCgroupfs(t *testing.T) {
	driver, systemd := selectDriverHost(ModeV2, true, map[string]bool{"cpu": true, "cpuset": true})
	if driver != DriverCgroupfs || systemd {
		t.Fatalf("nested policy = (%s, systemd=%t), want (cgroupfs, false)", driver, systemd)
	}
}

func TestValidatePreflightSkipsBareMetalSystemdDriver(t *testing.T) {
	policy := &CgroupPolicy{
		Mode:                 ModeV2,
		Driver:               DriverSystemd,
		Nested:               false,
		DelegatedControllers: []string{"memory", "pids"},
	}
	if err := ValidatePreflight(policy); err != nil {
		t.Fatalf("expected no preflight error for systemd driver, got %v", err)
	}
}

func TestValidatePreflightSkipsBareMetalCgroupfs(t *testing.T) {
	policy := &CgroupPolicy{
		Mode:                 ModeV2,
		Driver:               DriverCgroupfs,
		Nested:               false,
		DelegatedControllers: []string{"memory", "pids"},
	}
	if err := ValidatePreflight(policy); err != nil {
		t.Fatalf("expected no preflight error for bare-metal cgroupfs, got %v", err)
	}
}

func TestValidatePreflightSkipsV1(t *testing.T) {
	policy := &CgroupPolicy{
		Mode:                 ModeV1,
		Nested:               true,
		DelegatedControllers: []string{},
	}
	if err := ValidatePreflight(policy); err != nil {
		t.Fatalf("expected no preflight error for v1, got %v", err)
	}
}

func TestValidatePreflightMissingCPU(t *testing.T) {
	policy := &CgroupPolicy{
		Mode:                 ModeV2,
		Nested:               true,
		DelegatedControllers: []string{"memory", "pids"},
	}
	err := ValidatePreflight(policy)
	if err == nil {
		t.Fatal("expected delegation error")
	}
	var del *ErrDelegation
	if !errors.As(err, &del) {
		t.Fatalf("expected ErrDelegation, got %T: %v", err, err)
	}
	if del.Controller != "cpu" || !del.Nested {
		t.Fatalf("unexpected delegation error: %+v", del)
	}
}

type fakeDirInfo struct{}

func (fakeDirInfo) Name() string       { return "dockerenv" }
func (fakeDirInfo) Size() int64        { return 0 }
func (fakeDirInfo) Mode() os.FileMode  { return os.ModeDir }
func (fakeDirInfo) ModTime() time.Time { return time.Time{} }
func (fakeDirInfo) IsDir() bool        { return true }
func (fakeDirInfo) Sys() any           { return nil }
