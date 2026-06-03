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

MS_NAME="engine-switch-ms"
CLEANUP_POLL="${ENGINE_SWITCH_CLEANUP_POLL:-15}"

log_step "Engine switch test: ${SWITCH}"

"${SCRIPT_DIR}/vm-install.sh" --vm-name="${VM_NAME}" --start-engine="${START_ENGINE}"

scp -F "${HOME}/.lima/${VM_NAME}/ssh.config" -q \
    "${SCRIPT_DIR}/fixtures/engine-switch-ms.yaml" \
    "lima-${VM_NAME}:/tmp/engine-switch-ms.yaml"

assert_contains "MS deploy succeeds" "applied successfully" \
    R "edgelet deploy -f /tmp/engine-switch-ms.yaml"

assert_contains "MS running before switch" "engine-switch-ms" \
    R "edgelet ms ls"

assert_contains "pendingRestart false before switch" "false" \
    R "edgelet system status -o json | jq -r '.[\"runtime.pendingRestart\"]'"

assert_ok "config change to ${TARGET_ENGINE}" \
    R "set -euo pipefail
for i in \$(seq 1 30); do
  if edgelet config --container-engine ${TARGET_ENGINE}; then
    exit 0
  fi
  sleep 2
done
edgelet config --container-engine ${TARGET_ENGINE}"

assert_ok "pendingRestart true after config change" \
    R "set -euo pipefail
for i in \$(seq 1 15); do
  if [[ \$(edgelet system status -o json 2>/dev/null | jq -r '.[\"runtime.pendingRestart\"]') == 'true' ]]; then
    exit 0
  fi
  sleep 1
done
exit 1"

if [[ "${START_ENGINE}" == "edgelet" ]]; then
    assert_ok "switch MS runtime removed before restart (edgelet engine)" \
        R "set -euo pipefail
MS='${MS_NAME}'
POLL=${CLEANUP_POLL}
ok=0
for i in \$(seq 1 \"\${POLL}\"); do
  if python3 -c \"
import os, sqlite3, subprocess, sys
ms = sys.argv[1]
sock = '/run/edgelet/containerd.sock'
if not os.path.exists(sock):
    sys.exit(0)
c = sqlite3.connect('/var/lib/edgelet/edgelet.db')
rows = c.execute('select local_uuid from local_deployed_microservices where microservice_name=?', (ms,)).fetchall()
if not rows:
    sys.exit(0)
listed = subprocess.run(['ctr', '--address', sock, '-n', 'k8s.io', 'containers', 'list', '-q'], capture_output=True, text=True)
for cid in listed.stdout.split():
    cid = cid.strip()
    if not cid:
        continue
    info = subprocess.run(['ctr', '--address', sock, '-n', 'k8s.io', 'containers', 'info', cid], capture_output=True, text=True)
    for (uuid,) in rows:
        if uuid in info.stdout:
            sys.exit(1)
sys.exit(0)
\" \"\${MS}\"; then
    ok=1
    break
  fi
  sleep 1
done
test \"\${ok}\" -eq 1"
else
    assert_ok "switch MS runtime removed before restart (docker engine)" \
        R "set -euo pipefail
MS='${MS_NAME}'
POLL=${CLEANUP_POLL}
ok=0
for i in \$(seq 1 \"\${POLL}\"); do
  if python3 -c \"
import sqlite3, subprocess, sys
ms = sys.argv[1]
c = sqlite3.connect('/var/lib/edgelet/edgelet.db')
rows = c.execute('select local_uuid from local_deployed_microservices where microservice_name=?', (ms,)).fetchall()
for (uuid,) in rows:
    r = subprocess.run(['docker', 'ps', '-q', f'--filter=label=edgelet.iofog.org/microservice-uid={uuid}'], capture_output=True, text=True)
    if r.stdout.strip():
        sys.exit(1)
sys.exit(0)
\" \"\${MS}\"; then
    ok=1
    break
  fi
  sleep 1
done
test \"\${ok}\" -eq 1"
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
    R "edgelet system status -o json | jq -r '.[\"runtime.pendingRestart\"]'"

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
