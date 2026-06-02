#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

VM_NAME="edgelet-engine-lifecycle"
SWITCH="edgelet-to-docker"
for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --switch=*) SWITCH="${arg#*=}" ;;
    esac
done

case "${SWITCH}" in
    edgelet-to-docker) START_ENGINE="edgelet"; TARGET_ENGINE="docker" ;;
    docker-to-edgelet) START_ENGINE="docker"; TARGET_ENGINE="edgelet" ;;
    *) die "Unknown switch case: ${SWITCH}" ;;
esac

R() { echo "$*" | limactl --tty=false shell "${VM_NAME}" -- sudo bash; }

log_step "Engine switch test: ${SWITCH}"

"${SCRIPT_DIR}/vm-install.sh" --vm-name="${VM_NAME}" --start-engine="${START_ENGINE}"

R "docker pull docker.io/library/alpine:3.19"

scp -F "${HOME}/.lima/${VM_NAME}/ssh.config" -q \
    "${SCRIPT_DIR}/fixtures/engine-switch-ms.yaml" \
    "lima-${VM_NAME}:/tmp/engine-switch-ms.yaml"

assert_contains "MS deploy succeeds" "applied successfully" \
    R "edgelet deploy -f /tmp/engine-switch-ms.yaml"

assert_contains "MS running before switch" "engine-switch-ms" \
    R "edgelet ms ls"

assert_contains "pendingRestart false before switch" "false" \
    R "edgelet system status | grep runtime.pendingRestart"

R "edgelet config --container-engine ${TARGET_ENGINE}"

assert_contains "pendingRestart after config change" "true" \
    R "edgelet system status | grep runtime.pendingRestart"

if [[ "${START_ENGINE}" == "edgelet" ]]; then
    assert_ok "containers removed before restart (edgelet engine)" \
        R "! test -S /run/edgelet/containerd.sock || ! ctr --address /run/edgelet/containerd.sock -n k8s.io containers list -q 2>/dev/null | grep -q ."
else
    assert_ok "containers removed before restart (docker engine)" \
        R "! docker ps -q 2>/dev/null | grep -q ."
fi

assert_contains "local deploy spec retained in DB" "engine-switch-ms" \
    R "python3 -c \"import sqlite3; c=sqlite3.connect('/var/lib/edgelet/edgelet.db'); print(c.execute('select microservice_name from local_deployed_microservices').fetchall())\""

R "systemctl restart edgelet"

for i in $(seq 1 30); do
    if R "edgelet ms ls --source local 2>/dev/null | grep -q engine-switch-ms"; then
        break
    fi
    sleep 2
done

assert_contains "pendingRestart cleared after restart" "false" \
    R "edgelet system status | grep runtime.pendingRestart"

assert_contains "runtime.engine matches target" "${TARGET_ENGINE}" \
    R "edgelet system status | grep runtime.engine"

assert_contains "MS recreated on new engine" "engine-switch-ms" \
    R "edgelet ms ls --source local"

if [[ "${TARGET_ENGINE}" == "docker" ]]; then
    for i in $(seq 1 30); do
        if R "edgelet ms ls --source local 2>/dev/null | grep engine-switch-ms | grep -qi running"; then
            break
        fi
        sleep 2
    done
    assert_contains "MS running on docker engine" "running" \
        R "edgelet ms ls --source local | grep engine-switch-ms"
    assert_ok "MS container visible in Docker" \
        R "docker ps -q | grep -q ."

    log_step "Docker restart storm (local MS survives daemon restarts)"

    STORM_CYCLES="${DOCKER_RESTART_STORM_CYCLES:-10}"
    MS_NAME="engine-switch-ms"

    assert_contains "MS running before restart storm" "running" \
        R "edgelet ms ls --source local | grep ${MS_NAME}"

    assert_ok "restart storm: ${STORM_CYCLES} systemctl restart cycles" \
        R "set -euo pipefail
MS_NAME='${MS_NAME}'
CYCLES=${STORM_CYCLES}
for i in \$(seq 1 \"\${CYCLES}\"); do
  start_ts=\$(date +%s)
  systemctl restart edgelet
  ok=0
  for j in \$(seq 1 60); do
    if systemctl is-active --quiet edgelet && edgelet system status >/dev/null 2>&1; then
      ok=1
      break
    fi
    sleep 1
  done
  test \"\${ok}\" -eq 1
  elapsed=\$(( \$(date +%s) - start_ts ))
  test \"\${elapsed}\" -le 75
done"

    assert_ok "no SHUTDOWN_DRAIN_TIMEOUT during restart storm" \
        R "! journalctl -u edgelet -n 400 --no-pager | grep -q SHUTDOWN_DRAIN_TIMEOUT"

    for i in $(seq 1 30); do
        if R "edgelet ms ls --source local 2>/dev/null | grep ${MS_NAME} | grep -qi running"; then
            break
        fi
        sleep 2
    done

    assert_contains "MS running after restart storm" "running" \
        R "edgelet ms ls --source local | grep ${MS_NAME}"

    assert_ok "MS not stuck_in_restart after restart storm" \
        R "! edgelet ms ls --source local | grep ${MS_NAME} | grep -qi stuck_in_restart"

    assert_ok "MS not exiting after restart storm" \
        R "! edgelet ms ls --source local | grep ${MS_NAME} | grep -qi exiting"

    assert_ok "docker container still present" \
        R "set +e; ok=0; for i in \$(seq 1 15); do cid=\$(edgelet ms ls --source local 2>/dev/null | awk '/${MS_NAME}/ {print \$5}'); if [ -n \"\${cid}\" ] && [ -n \"\$(docker ps -q --filter id=\${cid})\" ]; then ok=1; break; fi; sleep 2; done; test \"\${ok}\" -eq 1"

    assert_ok "local failure_count not at stuck threshold" \
        R "python3 -c \"
import sqlite3
c=sqlite3.connect('/var/lib/edgelet/edgelet.db')
row=c.execute(
  \\\"select failure_count, runtime_state from local_deployed_microservices where microservice_name='${MS_NAME}'\\\"
).fetchone()
assert row is not None, 'missing local deploy row'
fc, rs = row
assert fc < 5, f'failure_count={fc} runtime_state={rs}'
assert rs == 'running', f'runtime_state={rs}'
\""
else
    assert_ok "fat runtime extracted for edgelet engine" \
        R "test -x /var/lib/edgelet/data/current/bin/edgelet"
fi

if (( TESTS_FAILED > 0 )); then
    die "${TESTS_FAILED} assertion(s) failed for ${SWITCH}"
fi
log_success "Engine switch ${SWITCH} passed"
