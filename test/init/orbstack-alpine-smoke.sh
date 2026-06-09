#!/usr/bin/env bash
# Manual smoke for OrbStack Alpine + OpenRC embedded engine (not CI).
#
# Prerequisites:
#   - OrbStack machine with Alpine (e.g. alpine2)
#   - edgelet installed from local build or release
#   - SSH/shell access as root
#
# Usage (on the OrbStack Alpine host):
#   edgelet cgroup-preflight
#   rc-service edgelet-containerd start
#   rc-service edgelet start
#   edgelet system status -o json | jq '{cgroupMode,cgroupDriver,cgroupNested,cgroupDelegatedControllers,runtime:."runtime.engineReady"}'
#
# Cold-boot gate: reboot first; do not run manual cgroup reparent commands.

set -euo pipefail

echo "=== OrbStack Alpine openrc smoke (manual) ==="
rc-status | grep -E 'edgelet-cgroup-prep|edgelet-containerd|edgelet' || true
edgelet cgroup-preflight
rc-service edgelet-containerd start
rc-service edgelet start
edgelet system status -o json | jq '{cgroupMode,cgroupDriver,cgroupNested,cgroupDelegatedControllers,runtime:."runtime.engineReady"}'
echo "OK"
