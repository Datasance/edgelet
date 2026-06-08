#!/usr/bin/env bash
# Shared host checks for Ubuntu hybrid cgroup v1 (systemd.unified_cgroup_hierarchy=0).
# Sourced by vm-test-cgroup-v1.sh; keep in sync with lima-ubuntu-v1.yaml probe.

# cgroup_v1_hybrid_host_ready — exit 0 when the kernel exposes v1 controller hierarchies.
cgroup_v1_hybrid_host_ready() {
    grep -q 'systemd.unified_cgroup_hierarchy=0' /proc/cmdline
    ! stat -fc %T /sys/fs/cgroup/ | grep -qx cgroup2fs
    test -d /sys/fs/cgroup/cpu
}
