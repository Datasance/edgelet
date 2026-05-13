#!/usr/bin/env bash
# test/embedded/vm-test.sh
#
# Runs the full embedded-containerd integration test suite inside the Lima VM.
# Tests are grouped into 5 phases:
#
#   Phase 1 — Extracted embedded binaries
#   Phase 2 — containerd socket & health
#   Phase 3 — CNI network configuration
#   Phase 4 — LocalAPI v3 + CLI microservice operations
#   Phase 5 — Runtime prerequisites
#   Phase 6 — CLI integration
#
# Usage:
#   ./test/embedded/vm-test.sh [--vm-name=iofog-test]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

VM_NAME="iofog-test"
for arg in "$@"; do
    case "${arg}" in --vm-name=*) VM_NAME="${arg#*=}" ;; esac
done

# Shorthand for running commands in VM as root.
# Pipe via stdin to avoid Lima's 9p mount caching — never use file-path args.
R() { echo "$*" | limactl --tty=false shell "${VM_NAME}" -- sudo bash; }
# Run as non-root (for CLI tests)
U() { echo "$*" | limactl --tty=false shell "${VM_NAME}" -- bash; }

echo ""
echo "======================================================================"
echo "  ioFog Embedded Containerd Integration Tests"
echo "  VM: ${VM_NAME}"
echo "======================================================================"

###############################################################################
# Phase 1 — Extracted embedded binaries
###############################################################################
log_step "Phase 1: Extracted embedded binaries"

assert_ok "containerd-shim-runc-v2 extracted" \
    R "test -x /var/lib/iofog-agent-containerd/bin/containerd-shim-runc-v2"

assert_ok "runc extracted" \
    R "test -x /var/lib/iofog-agent-containerd/bin/runc"

assert_ok "CNI bridge plugin extracted" \
    R "test -x /var/lib/iofog-agent-containerd/cni/plugins/bridge"

assert_ok "CNI host-local plugin extracted" \
    R "test -x /var/lib/iofog-agent-containerd/cni/plugins/host-local"

assert_ok "CNI portmap plugin extracted" \
    R "test -x /var/lib/iofog-agent-containerd/cni/plugins/portmap"

assert_ok "CNI loopback plugin extracted" \
    R "test -x /var/lib/iofog-agent-containerd/cni/plugins/loopback"

# spin shim is optional (not available on riscv64)
if R "test -f /var/lib/iofog-agent-containerd/bin/containerd-shim-spin" 2>/dev/null; then
    assert_ok "containerd-shim-spin extracted (spin/WASM support)" \
        R "test -x /var/lib/iofog-agent-containerd/bin/containerd-shim-spin"
else
    log_info "containerd-shim-spin not present (expected on riscv64 or before download)"
fi

###############################################################################
# Phase 2 — containerd socket & health
###############################################################################
log_step "Phase 2: containerd socket and health"

assert_ok "containerd config.toml written" \
    R "test -f /var/lib/iofog-agent-containerd/config.toml"

assert_ok "containerd config has iofog socket" \
    R "grep -q '/run/iofog-agent/containerd.sock' /var/lib/iofog-agent-containerd/config.toml"

assert_ok "containerd socket exists" \
    R "test -S /run/iofog-agent/containerd.sock"

assert_ok "iofog-agentd service is active" \
    R "systemctl is-active iofog-agentd"

assert_contains "iofog-agent status endpoint is reachable" "iofogDaemon" \
    R "iofog-agent system status"

###############################################################################
# Phase 3 — CNI network configuration
###############################################################################
log_step "Phase 3: CNI network configuration"

assert_ok "Managed CNI conflist written" \
    R "test -f /var/lib/iofog-agent-containerd/cni/conf/managed/10-iofog.conflist"

assert_ok "Managed CNI conflist has iofog network name" \
    R "jq -e '.name == \"iofog\"' /var/lib/iofog-agent-containerd/cni/conf/managed/10-iofog.conflist"

assert_ok "Managed CNI conflist has bridge plugin" \
    R "jq -e '.plugins[0].type == \"bridge\"' /var/lib/iofog-agent-containerd/cni/conf/managed/10-iofog.conflist"

assert_ok "Managed CNI conflist has bridge name iofog0" \
    R "jq -e '.plugins[0].bridge == \"iofog0\"' /var/lib/iofog-agent-containerd/cni/conf/managed/10-iofog.conflist"

assert_ok "Managed CNI conflist has portmap plugin" \
    R "jq -e '.plugins[] | select(.type==\"portmap\")' /var/lib/iofog-agent-containerd/cni/conf/managed/10-iofog.conflist"

assert_ok "Local CNI conflist written" \
    R "test -f /var/lib/iofog-agent-containerd/cni/conf/local/11-iofog-local.conflist"

assert_ok "Local CNI conflist has iofog-local network name" \
    R "jq -e '.name == \"iofog-local\"' /var/lib/iofog-agent-containerd/cni/conf/local/11-iofog-local.conflist"

assert_ok "Local CNI conflist has bridge name iofog-local0" \
    R "jq -e '.plugins[0].bridge == \"iofog-local0\"' /var/lib/iofog-agent-containerd/cni/conf/local/11-iofog-local.conflist"

assert_ok "Managed CNI system symlink created" \
    R "test -L /etc/cni/net.d/10-iofog.conflist"

assert_ok "Local CNI system symlink created" \
    R "test -L /etc/cni/net.d/11-iofog-local.conflist"

###############################################################################
# Phase 4 — LocalAPI v3 + CLI microservice operations
###############################################################################
log_step "Phase 4: LocalAPI v3 and CLI operations"

assert_contains "ms ps returns table headers" "APPLICATIONNAME" \
    R "iofog-agent ms ps"

assert_contains "auth whoami returns claims payload" "\"claims\"" \
    R "iofog-agent auth whoami"

assert_ok "create temporary local deploy manifest" \
    R "cat >/tmp/iofog-local-ms.yaml <<'EOF'
apiVersion: iofog.org/v3
kind: Microservice
metadata:
  name: local-test-ms
spec:
  images:
    x86: docker.io/library/alpine:3.19
    arm: docker.io/library/alpine:3.19
  container:
    hostNetworkMode: false
    isPrivileged: false
    commands:
      - /bin/sh
      - -lc
      - sleep 10
  schedule: 50
EOF"

assert_contains "deploy -f submits manifest via CLI" "microservice manifest applied successfully" \
    R "iofog-agent deploy -f /tmp/iofog-local-ms.yaml"

###############################################################################
# Phase 5 — Runtime prerequisites
###############################################################################
log_step "Phase 5: Runtime prerequisites"

# Check IP forwarding is enabled
assert_ok "IP forwarding enabled" \
    R "cat /proc/sys/net/ipv4/ip_forward | grep -q 1"

# Check runc is functional
assert_ok "runc is executable and reports version" \
    R "/var/lib/iofog-agent-containerd/bin/runc --version"

###############################################################################
# Phase 6 — CLI integration
###############################################################################
log_step "Phase 6: CLI integration"

assert_ok "iofog-agent binary is executable" \
    R "test -x /usr/local/bin/iofog-agent"

# assert_contains "iofog-agent version returns version string" "ioFog" \
#     R "iofog-agent version"

assert_contains "iofog-agent info shows containerEngine=iofog" "iofog" \
    R "iofog-agent info 2>/dev/null || iofog-agent info"

assert_contains "iofog-agent config get returns container engine field" "\"containerEngine\"" \
    R "iofog-agent config get"

assert_ok "iofog-agent config set containerEngine iofog accepted" \
    R "iofog-agent config set containerEngine iofog"

assert_contains "config get reflects engine=iofog" "\"containerEngine\": \"iofog\"" \
    R "iofog-agent config get"

###############################################################################
# Summary
###############################################################################
print_summary
