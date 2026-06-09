package cgroups

import "testing"

func TestIsMachineRootCgroupPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"", true},
		{"/.lxc", true},
		{"/.lxc/init", true},
		{".lxc/init", true},
		{"/init.scope", true},
		{"/docker/abc/container", false},
		{"/lxc/payload/uid_1000/pid_12345", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := IsMachineRootCgroupPath(tt.path); got != tt.want {
				t.Fatalf("IsMachineRootCgroupPath(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

func TestCgroupPathIndicatesNested(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "edgelet split data plane unit",
			path: "/system.slice/edgelet-containerd.service/edgelet-containerd.scope",
			want: false,
		},
		{
			name: "edgelet control plane unit",
			path: "/system.slice/edgelet.service/edgelet.scope",
			want: false,
		},
		{
			name: "orbstack machine root",
			path: "/.lxc",
			want: false,
		},
		{
			name: "orbstack cgroup prep staging",
			path: "/.lxc/init",
			want: false,
		},
		{
			name: "orbstack cgroup prep staging relative",
			path: ".lxc/init",
			want: false,
		},
		{
			name: "wsl init scope",
			path: "/init.scope",
			want: false,
		},
		{
			name: "host root",
			path: "/",
			want: false,
		},
		{
			name: "openrc service cgroup",
			path: "openrc.edgelet-containerd",
			want: false,
		},
		{
			name: "docker nested",
			path: "/docker/abc123def4567890123456789012345678901234567890123456789012345678/container",
			want: true,
		},
		{
			name: "libpod nested",
			path: "/machine.slice/libpod-abc123.scope/cri-containerd-abc.scope",
			want: true,
		},
		{
			name: "kubepods nested",
			path: "/kubepods/burstable/pod1234/cri-containerd-5678",
			want: true,
		},
		{
			name: "foreign containerd service",
			path: "/system.slice/containerd.service/containerd.scope",
			want: true,
		},
		{
			name: "edgelet containerd child cgroup under split unit",
			path: "/system.slice/edgelet-containerd.service/containerd",
			want: false,
		},
		{
			name: "lxc payload nested",
			path: "/lxc/payload/uid_1000/pid_12345",
			want: true,
		},
		{
			name: "podman nested",
			path: "/machine.slice/libpod-conmon-abc.scope",
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := cgroupPathIndicatesNested(tt.path); got != tt.want {
				t.Fatalf("cgroupPathIndicatesNested(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}
