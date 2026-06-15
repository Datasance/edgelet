//go:build linux && cgo

package cgroups

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetectModeFromMount(t *testing.T) {
	mode, mount, _ := detectMode()
	if mode == ModeUnknown {
		t.Fatalf("expected detectable cgroup mode, got unknown (mount=%q)", mount)
	}
	if mount == "" {
		t.Fatal("expected non-empty unified mountpoint")
	}
}

func TestDetectNestedDockerenv(t *testing.T) {
	prevStat := statFn
	t.Cleanup(func() { statFn = prevStat })
	statFn = func(name string) (os.FileInfo, error) {
		if name == "/.dockerenv" {
			return fakeDirInfo{}, nil
		}
		return prevStat(name)
	}
	if !detectNested() {
		t.Fatal("expected nested=true when /.dockerenv exists")
	}
}

func TestSelectDriverNestedPrefersCgroupfs(t *testing.T) {
	driver, systemd := selectDriver(ModeV2, true, map[string]bool{"cpu": true, "cpuset": true})
	if driver != DriverCgroupfs || systemd {
		t.Fatalf("nested policy = (%s, systemd=%t), want (cgroupfs, false)", driver, systemd)
	}
}

func TestSelectDriverCgroupfsWhenRootCpusetMissing(t *testing.T) {
	prevStat := statFn
	prevComm := pid1CommFn
	prevRoot := rootHasControllerFn
	t.Cleanup(func() {
		statFn = prevStat
		pid1CommFn = prevComm
		rootHasControllerFn = prevRoot
	})
	statFn = func(name string) (os.FileInfo, error) {
		if name == "/run/systemd/system" {
			return fakeDirInfo{}, nil
		}
		return prevStat(name)
	}
	pid1CommFn = func() (string, error) { return "systemd", nil }
	rootHasControllerFn = func(string) bool { return false }
	t.Setenv("INVOCATION_ID", "test-invocation")

	driver, systemd := selectDriver(ModeV2, false, map[string]bool{"cpu": true, "cpuset": true})
	if driver != DriverCgroupfs || systemd {
		t.Fatalf("policy without root cpuset = (%s, systemd=%t), want (cgroupfs, false)", driver, systemd)
	}
}

func TestSelectDriverSystemdOnV2ServiceWithoutCPUDelegated(t *testing.T) {
	prevStat := statFn
	prevComm := pid1CommFn
	prevRoot := rootHasControllerFn
	t.Cleanup(func() {
		statFn = prevStat
		pid1CommFn = prevComm
		rootHasControllerFn = prevRoot
	})
	statFn = func(name string) (os.FileInfo, error) {
		if name == "/run/systemd/system" {
			return fakeDirInfo{}, nil
		}
		return prevStat(name)
	}
	pid1CommFn = func() (string, error) { return "systemd", nil }
	rootHasControllerFn = func(name string) bool { return name == "cpuset" }
	t.Setenv("INVOCATION_ID", "test-invocation")

	// host gate: root cpuset + INVOCATION_ID → systemd driver; crun stays cgroupfs.
	driver, systemd := selectDriver(ModeV2, false, map[string]bool{"memory": true, "pids": true})
	if driver != DriverSystemd || systemd {
		t.Fatalf("systemd service policy = (%s, systemd=%t), want (systemd, false)", driver, systemd)
	}
}

func TestSelectDriverSystemdOnV2Host(t *testing.T) {
	prevStat := statFn
	prevComm := pid1CommFn
	prevRoot := rootHasControllerFn
	t.Cleanup(func() {
		statFn = prevStat
		pid1CommFn = prevComm
		rootHasControllerFn = prevRoot
	})
	statFn = func(name string) (os.FileInfo, error) {
		if name == "/run/systemd/system" {
			return fakeDirInfo{}, nil
		}
		return prevStat(name)
	}
	pid1CommFn = func() (string, error) { return "systemd", nil }
	rootHasControllerFn = func(name string) bool { return name == "cpuset" }
	t.Setenv("INVOCATION_ID", "test-invocation")

	driver, systemd := selectDriver(ModeV2, false, map[string]bool{"cpu": true})
	if driver != DriverSystemd || systemd {
		t.Fatalf("host systemd policy = (%s, systemd=%t), want (systemd, false)", driver, systemd)
	}
}

func TestCgroupPathsSystemdFormat(t *testing.T) {
	agent, ctd := cgroupPaths(ModeV2, DriverSystemd)
	if agent != "/edgelet/agent" || ctd != "/edgelet/agent/containerd" {
		t.Fatalf("unexpected paths: agent=%q containerd=%q", agent, ctd)
	}
}

func TestCgroupPathsCgroupfsFormat(t *testing.T) {
	agent, ctd := cgroupPaths(ModeV2, DriverCgroupfs)
	if agent != "/edgelet/agent" || ctd != "/edgelet/agent/containerd" {
		t.Fatalf("unexpected paths: agent=%q containerd=%q", agent, ctd)
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
	if !asErrDelegation(err, &del) {
		t.Fatalf("expected ErrDelegation, got %T: %v", err, err)
	}
	if del.Controller != "cpu" || !del.Nested {
		t.Fatalf("unexpected delegation error: %+v", del)
	}
}

func TestMapRuntimeError(t *testing.T) {
	policy := &CgroupPolicy{Nested: true, Mode: ModeV2}
	err := MapRuntimeError(os.ErrInvalid, policy)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected passthrough, got %v", err)
	}
	mapped := MapRuntimeError(errors.New("cri failed: controller cpu is not available"), policy)
	var del *ErrDelegation
	if !asErrDelegation(mapped, &del) {
		t.Fatalf("expected mapped delegation error, got %v", mapped)
	}
}

func TestCheckDelegatedControllersUsesSelfCgroup(t *testing.T) {
	controllers := checkDelegatedControllers(defaultUnifiedMount)
	if len(controllers) == 0 && Mode(detectModeString()) != ModeV1 {
		t.Fatal("expected delegated controllers on unified host, got none")
	}
}

func TestUniqueSorted(t *testing.T) {
	got := uniqueSorted([]string{"pids", "cpu", "cpu", "memory"})
	want := "cpu,memory,pids"
	if joinControllers(got) != want {
		t.Fatalf("uniqueSorted = %v (%q), want %q", got, joinControllers(got), want)
	}
}

func TestCurrentUnifiedCgroupPath(t *testing.T) {
	path, err := currentUnifiedCgroupPath(defaultUnifiedMount)
	if err != nil {
		t.Fatalf("currentUnifiedCgroupPath failed: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty unified cgroup path")
	}
	_ = filepath.Base(path)
}

func TestNormalizeRelUnifiedPath(t *testing.T) {
	cases := map[string]string{
		"/":        "",
		"":         "",
		"/edgelet": "edgelet",
		"edgelet":  "edgelet",
		"/a/b":     "a/b",
	}
	for in, want := range cases {
		if got := normalizeRelUnifiedPath(in); got != want {
			t.Fatalf("normalizeRelUnifiedPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildSubtreeEnableLine(t *testing.T) {
	got := buildSubtreeEnableLine([]string{"cpu", "memory", "pids"})
	want := "+cpu +memory +pids"
	if got != want {
		t.Fatalf("buildSubtreeEnableLine = %q, want %q", got, want)
	}
}

func TestJoinRelUnifiedPath(t *testing.T) {
	if got := joinRelUnifiedPath("", "init"); got != "init" {
		t.Fatalf("joinRelUnifiedPath empty root = %q, want init", got)
	}
	if got := joinRelUnifiedPath("edgelet", "init"); got != "edgelet/init" {
		t.Fatalf("joinRelUnifiedPath edgelet = %q, want edgelet/init", got)
	}
}

type fakeDirInfo struct{}

func (fakeDirInfo) Name() string       { return "dockerenv" }
func (fakeDirInfo) Size() int64        { return 0 }
func (fakeDirInfo) Mode() os.FileMode  { return os.ModeDir }
func (fakeDirInfo) ModTime() time.Time { return time.Time{} }
func (fakeDirInfo) IsDir() bool        { return true }
func (fakeDirInfo) Sys() any           { return nil }

func detectModeString() string {
	mode, _, _ := detectMode()
	return string(mode)
}

func asErrDelegation(err error, target **ErrDelegation) bool {
	return errors.As(err, target)
}

func TestMachineRootDelegationSatisfied(t *testing.T) {
	const mount = "/sys/fs/cgroup"

	tests := []struct {
		name      string
		rootSub   string
		lxcExists bool
		lxcSub    string
		want      bool
	}{
		{
			name:      "root and lxc delegated",
			rootSub:   "cpu memory pids",
			lxcExists: true,
			lxcSub:    "cpu memory pids",
			want:      true,
		},
		{
			name:      "root missing cpu",
			rootSub:   "memory pids",
			lxcExists: true,
			lxcSub:    "cpu memory pids",
			want:      false,
		},
		{
			name:      "lxc missing pids",
			rootSub:   "cpu memory pids",
			lxcExists: true,
			lxcSub:    "cpu memory",
			want:      false,
		},
		{
			name:      "no lxc node bare root only",
			rootSub:   "cpu memory pids",
			lxcExists: false,
			want:      true,
		},
		{
			name:      "no lxc node root incomplete",
			rootSub:   "cpu",
			lxcExists: false,
			want:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			prevRead := readFileFn
			prevStat := statFn
			t.Cleanup(func() {
				readFileFn = prevRead
				statFn = prevStat
			})

			readFileFn = func(path string) ([]byte, error) {
				switch path {
				case filepath.Join(mount, "cgroup.subtree_control"):
					return []byte(tt.rootSub), nil
				case filepath.Join(mount, ".lxc", "cgroup.subtree_control"):
					return []byte(tt.lxcSub), nil
				default:
					return prevRead(path)
				}
			}
			statFn = func(name string) (os.FileInfo, error) {
				if name == filepath.Join(mount, ".lxc", "cgroup.controllers") {
					if tt.lxcExists {
						return fakeDirInfo{}, nil
					}
					return nil, os.ErrNotExist
				}
				return prevStat(name)
			}

			if got := machineRootDelegationSatisfied(mount); got != tt.want {
				t.Fatalf("machineRootDelegationSatisfied() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestPrepareMachineRootDelegationSkipsWhenSatisfied(t *testing.T) {
	const mount = "/sys/fs/cgroup"
	prevRead := readFileFn
	prevWrite := writeFileFn
	prevStat := statFn
	t.Cleanup(func() {
		readFileFn = prevRead
		writeFileFn = prevWrite
		statFn = prevStat
	})

	evacuateWrites := 0
	delegatedSubCtrl := []byte("cpu memory pids")
	readFileFn = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "cgroup.subtree_control") && strings.HasPrefix(path, mount) {
			return delegatedSubCtrl, nil
		}
		return prevRead(path)
	}
	statFn = func(name string) (os.FileInfo, error) {
		if name == filepath.Join(mount, ".lxc", "cgroup.controllers") {
			return fakeDirInfo{}, nil
		}
		return prevStat(name)
	}
	writeFileFn = func(name string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(name, "cgroup.procs") {
			evacuateWrites++
		}
		return prevWrite(name, data, perm)
	}

	policy := &CgroupPolicy{UnifiedMountpoint: mount}
	if err := prepareMachineRootDelegation(policy); err != nil {
		t.Fatalf("prepareMachineRootDelegation: %v", err)
	}
	if evacuateWrites != 0 {
		t.Fatalf("expected no cgroup.procs evacuate when delegation satisfied, got %d writes", evacuateWrites)
	}
}
