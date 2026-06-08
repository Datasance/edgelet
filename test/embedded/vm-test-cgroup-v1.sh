#!/usr/bin/env bash
# test/embedded/vm-test-cgroup-v1.sh
#
# Hybrid cgroup v1 gate. Prefer the master runner:
#   ./test/embedded/run-all-cgroup-v1.sh
#
# Manual:
#   limactl start --name=iofog-test-v1 test/embedded/lima-ubuntu-v1.yaml
#   ./test/embedded/vm-test-cgroup-v1.sh [--vm-name=iofog-test-v1]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"
source "${SCRIPT_DIR}/lib/cgroup-v1-host.sh"

VM_NAME="iofog-test-v1"
for arg in "$@"; do
    case "${arg}" in --vm-name=*) VM_NAME="${arg#*=}" ;; esac
done

log_step "cgroup v1/hybrid VM gate (${VM_NAME})"

if ! limactl list --json 2>/dev/null | jq -e "select(.name == \"${VM_NAME}\")" >/dev/null; then
    log_warn "VM ${VM_NAME} not found — create with: limactl start --name=${VM_NAME} ${SCRIPT_DIR}/lima-ubuntu-v1.yaml"
    exit 0
fi

R() { echo "$*" | limactl --tty=false shell "${VM_NAME}" -- sudo bash; }

assert_ok "host reports hybrid cgroup v1 (not pure v2)" \
    R "source ${REPO_ROOT}/test/embedded/lib/cgroup-v1-host.sh && cgroup_v1_hybrid_host_ready"

assert_ok "edgelet-containerd service active on v1 VM (split)" \
    R "systemctl is-active --quiet edgelet-containerd"

assert_ok "edgelet service active on v1 VM" \
    R "systemctl is-active --quiet edgelet"

assert_ok "split unit installed" \
    R "test -f /etc/systemd/system/edgelet-containerd.service"

assert_ok "data plane cgroup mode is v1 or hybrid (split install)" \
    R 'journalctl -u edgelet-containerd --no-pager | grep -Eq "cgroup mode=(v1|hybrid)"'

assert_contains "deploy manifest succeeds on hybrid/v1 host" "microservice manifest applied successfully" \
    R "set -e
cat >/tmp/iofog-v1-ms.yaml <<'EOF'
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: v1-test-ms
spec:
  image: docker.io/library/alpine:3.19
  registry: 1
  container:
    hostNetworkMode: false
    commands: [/bin/sh, -lc, 'sleep 600']
  schedule: 50
EOF
edgelet deploy -f /tmp/iofog-v1-ms.yaml"

print_summary
