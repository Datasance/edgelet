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

assert_ok "crun extracted" \
    R "test -x /var/lib/iofog-agent-containerd/bin/crun"

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

assert_ok "ms ps is reachable (table or empty state)" \
    R "set -e
out=\$(iofog-agent ms ps || true)
echo \"\${out}\" | grep -Eq 'MICROSERVICENAME|No microservices found.'"

assert_contains "auth whoami returns claims payload" "\"claims\"" \
    R "iofog-agent auth whoami"

assert_ok "seed/list local registries before deploy tests" \
    R "iofog-agent registry ls"

assert_ok "create temporary local deploy manifest" \
    R "cat >/tmp/iofog-local-ms.yaml <<'EOF'
apiVersion: iofog.org/v3
kind: Microservice
metadata:
  name: local-test-ms
spec:
  images:
    registry: 1
    x86: docker.io/library/alpine:3.19
    arm: docker.io/library/alpine:3.19
  container:
    hostNetworkMode: false
    isPrivileged: false
    commands:
      - /bin/sh
      - -lc
      - sleep 14000
  schedule: 50
EOF"

assert_contains "deploy -f submits manifest via CLI" "microservice manifest applied successfully" \
    R "iofog-agent deploy -f /tmp/iofog-local-ms.yaml"

assert_ok "create DNS probe workload A manifest" \
    R "cat >/tmp/iofog-local-dns-a.yaml <<'EOF'
apiVersion: iofog.org/v3
kind: Microservice
metadata:
  name: local-dns-a
spec:
  images:
    registry: 1
    x86: docker.io/library/busybox:1.36
    arm: docker.io/library/busybox:1.36
  container:
    hostNetworkMode: false
    isPrivileged: false
    commands:
      - /bin/sh
      - -lc
      - sleep 1200
  schedule: 50
EOF"

assert_ok "create DNS probe workload B manifest" \
    R "cat >/tmp/iofog-local-dns-b.yaml <<'EOF'
apiVersion: iofog.org/v3
kind: Microservice
metadata:
  name: local-dns-b
spec:
  images:
    registry: 1
    x86: docker.io/library/busybox:1.36
    arm: docker.io/library/busybox:1.36
  container:
    hostNetworkMode: false
    isPrivileged: false
    commands:
      - /bin/sh
      - -lc
      - sleep 1200
  schedule: 50
EOF"

assert_contains "deploy DNS probe workload A" "microservice manifest applied successfully" \
    R "iofog-agent deploy -f /tmp/iofog-local-dns-a.yaml"

assert_contains "deploy DNS probe workload B" "microservice manifest applied successfully" \
    R "iofog-agent deploy -f /tmp/iofog-local-dns-b.yaml"

assert_ok "discover DNS probe UUID selectors" \
    R "set -e
for i in \$(seq 1 30); do
  ps_out=\$(iofog-agent ms ps || true)
  dns_a_uuid=\$(echo \"\${ps_out}\" | awk '\$3==\"local-dns-a\"{print \$1; exit}')
  dns_b_uuid=\$(echo \"\${ps_out}\" | awk '\$3==\"local-dns-b\"{print \$1; exit}')
  if [ -n \"\${dns_a_uuid}\" ] && [ -n \"\${dns_b_uuid}\" ]; then
    cat >/tmp/pr6-dns-uuids.env <<EOF
DNS_A_UUID=\${dns_a_uuid}
DNS_B_UUID=\${dns_b_uuid}
EOF
    exit 0
  fi
  sleep 2
done
exit 1"

assert_ok "local DNS probe resolves peer from inside container" \
    R "set -e
source /tmp/pr6-dns-uuids.env
for i in \$(seq 1 20); do
  if iofog-agent ms exec \"\${DNS_A_UUID}\" -- nslookup local.local-dns-b >/dev/null 2>&1; then
    exit 0
  fi
  sleep 3
done
exit 1"

assert_ok "reserved agent alias resolves from local workload" \
    R "set -e
source /tmp/pr6-dns-uuids.env
iofog-agent ms exec \"\${DNS_A_UUID}\" -- nslookup iofog.default.svc.bridge.local >/dev/null 2>&1"

assert_ok "status exposes DNS operability fields" \
    R "set -e
out=\$(iofog-agent system status)
echo \"\${out}\" | grep -q 'dnsHealth'
echo \"\${out}\" | grep -q 'dnsScopeLocalListening'
echo \"\${out}\" | grep -q 'dnsRateLimitEnabled'"

assert_ok "metrics endpoint exposes DNS series" \
    R "set -e
if command -v curl >/dev/null 2>&1; then
  metrics=\$(curl -ksSf https://127.0.0.1:54321/metrics || curl --unix-socket /var/run/iofog-agentd.sock -sSf http://localhost/metrics)
  echo \"\${metrics}\" | grep -q 'iofog_dns_queries_total'
  echo \"\${metrics}\" | grep -q 'iofog_dns_forwarding_degraded'
  echo \"\${metrics}\" | grep -q 'iofog_dns_rate_limited_total'
else
  exit 1
fi"

assert_ok "airgapped forward-fail keeps internal local resolution working" \
    R "set -e
source /tmp/pr6-dns-uuids.env
orig_target=\$(readlink -f /etc/resolv.conf)
cp -a \"\${orig_target}\" /tmp/resolv.conf.pr6.bak
trap 'cat /tmp/resolv.conf.pr6.bak > \"\${orig_target}\"; rm -f /tmp/resolv.conf.pr6.bak' EXIT
before=\$(iofog-agent system status)
before_q=\$(echo \"\${before}\" | awk -F': ' '/dnsQueriesTotal/{print \$2}')
before_succ=\$(echo \"\${before}\" | awk -F': ' '/dnsSuccessTotal/{print \$2}')
before_ferr=\$(echo \"\${before}\" | awk -F': ' '/dnsForwardErrTotal/{print \$2}')
before_srv=\$(echo \"\${before}\" | awk -F': ' '/dnsServFailTotal/{print \$2}')
printf 'nameserver 203.0.113.1\noptions timeout:1 attempts:1\n' >\"\${orig_target}\"
set +e
iofog-agent ms exec \"\${DNS_A_UUID}\" -- nslookup example.com >/tmp/pr6-airgap-external.out 2>&1
iofog-agent ms exec \"\${DNS_A_UUID}\" -- nslookup local.local-dns-b >/tmp/pr6-airgap-internal.out 2>&1
set -e
after=\$(iofog-agent system status)
after_q=\$(echo \"\${after}\" | awk -F': ' '/dnsQueriesTotal/{print \$2}')
after_succ=\$(echo \"\${after}\" | awk -F': ' '/dnsSuccessTotal/{print \$2}')
after_ferr=\$(echo \"\${after}\" | awk -F': ' '/dnsForwardErrTotal/{print \$2}')
after_srv=\$(echo \"\${after}\" | awk -F': ' '/dnsServFailTotal/{print \$2}')
test \"\${after_q}\" -gt \"\${before_q}\"
test \"\${after_succ}\" -gt \"\${before_succ}\"
test \"\${after_ferr}\" -gt \"\${before_ferr}\"
test \"\${after_srv}\" -gt \"\${before_srv}\"
echo \"\${after}\" | grep -q 'dnsForwardingDegraded: true'
cat /tmp/resolv.conf.pr6.bak > \"\${orig_target}\"
rm -f /tmp/resolv.conf.pr6.bak
trap - EXIT"

###############################################################################
# Phase 5 — Runtime prerequisites
###############################################################################
log_step "Phase 5: Runtime prerequisites"

# Check IP forwarding is enabled
assert_ok "IP forwarding enabled" \
    R "cat /proc/sys/net/ipv4/ip_forward | grep -q 1"

# Check crun is functional
assert_ok "crun is executable and reports version" \
    R "/var/lib/iofog-agent-containerd/bin/crun --version"

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

# assert_ok "iofog-agent config set containerEngine iofog accepted" \
#     R "iofog-agent config -ce iofog"

# assert_contains "iofog-agent info shows containerEngine=iofog" "iofog" \
#     R "iofog-agent info 2>/dev/null || iofog-agent info"

###############################################################################
# Phase 7 — Chaos gates (restart storm + crash injection)
###############################################################################
log_step "Phase 7: Chaos gates"

assert_ok "restart storm converges across 10 systemctl restart cycles" \
    R "set -e
for i in \$(seq 1 10); do
  start_ts=\$(date +%s)
  systemctl restart iofog-agentd
  ok=0
  for j in \$(seq 1 60); do
    if systemctl is-active --quiet iofog-agentd && iofog-agent system status >/dev/null 2>&1; then
      ok=1
      break
    fi
    sleep 1
  done
  test \"\${ok}\" -eq 1
  elapsed=\$(( \$(date +%s) - start_ts ))
  # Keep restart bounded so lingering shims cannot hold the unit in final-sigterm for minutes.
  test \"\${elapsed}\" -le 75
done"

assert_ok "service leaves deactivating state after restart storm" \
    R "set -e
sub_state=\$(systemctl show -p SubState --value iofog-agentd)
test \"\${sub_state}\" != \"deactivating\""

assert_ok "journald has no forbidden startup signatures" \
    R "set -e
! journalctl -u iofog-agentd -n 800 --no-pager | grep -Eqi 'text file busy|ETXTBSY|Start request repeated too quickly'"

assert_ok "runtime child crash recovers within bounded window" \
    R "set -e
old_child=\$(pgrep -f -- '--iofog-containerd-child' | head -n1 || true)
test -n \"\${old_child}\"
kill -9 \"\${old_child}\" || true
ok=0
for i in \$(seq 1 150); do
  new_child=\$(pgrep -f -- '--iofog-containerd-child' | head -n1 || true)
  if [ -n \"\${new_child}\" ] && [ \"\${new_child}\" != \"\${old_child}\" ] && systemctl is-active --quiet iofog-agentd && iofog-agent system status >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep 1
done
test \"\${ok}\" -eq 1"

assert_ok "DNS convergence remains healthy after restart storm and crash recovery" \
    R "set -e
source /tmp/pr6-dns-uuids.env
for i in \$(seq 1 10); do
  iofog-agent ms exec \"\${DNS_A_UUID}\" -- nslookup local.local-dns-b >/dev/null 2>&1
done
status=\$(iofog-agent system status)
echo \"\${status}\" | grep -q 'dnsHealth: ready\\|dnsHealth: degraded'
echo \"\${status}\" | grep -q 'dnsForwardErrTotal'"

###############################################################################
# Summary
###############################################################################
print_summary
