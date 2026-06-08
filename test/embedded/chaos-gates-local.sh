#!/usr/bin/env bash
set -euo pipefail

# Local Linux chaos gate (non-systemd):
# - restart storm loop with bounded convergence
# - forbidden signature scan (ETXTBSY / start-limit lockout)
# - runtime child crash injection (watchdog-driven daemon termination)
#
# Intended for fast local/container validation before VM/systemd chaos gates.

cycles="${1:-10}"
start_timeout_seconds="${START_TIMEOUT_SECONDS:-30}"
graceful_stop_timeout_seconds="${GRACEFUL_STOP_TIMEOUT_SECONDS:-45}"
crash_recovery_timeout_seconds="${CRASH_RECOVERY_TIMEOUT_SECONDS:-130}"

forbidden_pattern='text file busy|etxtbsy|Start request repeated too quickly'
shim_pattern='containerd-shim-.*-address /run/edgelet/containerd.sock'

mkdir -p /etc/edgelet
cp "packaging/edgelet/etc/edgelet/config_full.yaml" "/etc/edgelet/config.yaml"
cp "packaging/edgelet/etc/edgelet/cert_new.crt" "/etc/edgelet/cert.crt"

echo "==> [PR6 local gate] embed pipeline + unified linux edgelet build"
make build-edgelet-local

if [[ ! -x "./build/edgelet" ]]; then
  echo "ERROR: missing build/edgelet after build"
  exit 1
fi

wait_until_ready() {
  local daemon_pid="$1"
  local deadline=$((SECONDS + start_timeout_seconds))
  while (( SECONDS < deadline )); do
    if ! kill -0 "${daemon_pid}" 2>/dev/null; then
      return 1
    fi
    if ./build/edgelet system status >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_dns_probe_ready() {
  local deadline=$((SECONDS + 90))
  while (( SECONDS < deadline )); do
    if [[ -f /tmp/pr6-local-dns-uuids.env ]]; then
      # shellcheck disable=SC1091
      source /tmp/pr6-local-dns-uuids.env
    fi
    if [[ -n "${DNS_A_UUID:-}" ]] && ./build/edgelet ms exec "${DNS_A_UUID}" -- nslookup edgelet.local-chaos-dns-b >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

ensure_dns_probe_manifests() {
  cat >/tmp/local-chaos-dns-a.yaml <<'EOF'
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: local-chaos-dns-a
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
EOF

  cat >/tmp/local-chaos-dns-b.yaml <<'EOF'
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: local-chaos-dns-b
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
EOF
}

run_dns_chaos_probe_burst() {
  local loops="${1:-6}"
  local failures=0
  if [[ -f /tmp/pr6-local-dns-uuids.env ]]; then
    # shellcheck disable=SC1091
    source /tmp/pr6-local-dns-uuids.env
  fi
  if [[ -z "${DNS_A_UUID:-}" ]]; then
    return 1
  fi
  for _ in $(seq 1 "${loops}"); do
    if ! ./build/edgelet ms exec "${DNS_A_UUID}" -- nslookup edgelet.local-chaos-dns-b >/dev/null 2>&1; then
      failures=$((failures + 1))
    fi
    sleep 1
  done
  if (( failures > 2 )); then
    return 1
  fi
  return 0
}

discover_dns_probe_uuids() {
  local deadline=$((SECONDS + 60))
  while (( SECONDS < deadline )); do
    ps_out="$(./build/edgelet ms ls || true)"
    dns_a_uuid="$(echo "${ps_out}" | awk '$3=="local-chaos-dns-a"{print $1; exit}')"
    dns_b_uuid="$(echo "${ps_out}" | awk '$3=="local-chaos-dns-b"{print $1; exit}')"
    if [[ -n "${dns_a_uuid}" && -n "${dns_b_uuid}" ]]; then
      cat >/tmp/pr6-local-dns-uuids.env <<EOF
DNS_A_UUID=${dns_a_uuid}
DNS_B_UUID=${dns_b_uuid}
EOF
      return 0
    fi
    sleep 2
  done
  return 1
}

stop_daemon() {
  local daemon_pid="$1"
  kill -TERM "${daemon_pid}" >/dev/null 2>&1 || true
  local deadline=$((SECONDS + graceful_stop_timeout_seconds))
  while (( SECONDS < deadline )); do
    if ! kill -0 "${daemon_pid}" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  kill -KILL "${daemon_pid}" >/dev/null 2>&1 || true
  return 1
}

assert_no_runtime_orphans() {
  local scope="$1"
  if pgrep -af -- "--edgelet-containerd-child" >/tmp/pr5-local-child-orphans.txt 2>&1; then
    echo "ERROR: orphan runtime child ${scope}"
    cat /tmp/pr5-local-child-orphans.txt
    exit 1
  fi
  if pgrep -af "${shim_pattern}" >/tmp/pr5-local-shim-orphans.txt 2>&1; then
    echo "ERROR: orphan runtime shim ${scope}"
    cat /tmp/pr5-local-shim-orphans.txt
    exit 1
  fi
}

echo "==> [PR6 local gate] restart storm (${cycles} cycles)"
dns_assets_created=0
for i in $(seq 1 "${cycles}"); do
  log_file="/tmp/pr5-local-restart-${i}.log"
  ./build/edgelet daemon >"${log_file}" 2>&1 &
  daemon_pid=$!

  if ! wait_until_ready "${daemon_pid}"; then
    echo "ERROR: cycle ${i} did not become ready"
    stop_daemon "${daemon_pid}" || true
    exit 1
  fi

  if [[ "${dns_assets_created}" -eq 0 ]]; then
    ensure_dns_probe_manifests
    ./build/edgelet deploy -f /tmp/local-chaos-dns-a.yaml >/dev/null 2>&1 || true
    ./build/edgelet deploy -f /tmp/local-chaos-dns-b.yaml >/dev/null 2>&1 || true
    if ! discover_dns_probe_uuids; then
      echo "ERROR: could not discover DNS probe UUID selectors"
      stop_daemon "${daemon_pid}" || true
      exit 1
    fi
    dns_assets_created=1
  fi

  if ! wait_dns_probe_ready; then
    echo "ERROR: DNS probe workloads did not become query-ready in cycle ${i}"
    stop_daemon "${daemon_pid}" || true
    exit 1
  fi

  if ! run_dns_chaos_probe_burst 6; then
    echo "ERROR: DNS in-container probes failed during cycle ${i}"
    stop_daemon "${daemon_pid}" || true
    exit 1
  fi

  if ! ./build/edgelet system status | grep -q 'dnsHealth'; then
    echo "ERROR: DNS health field missing from system status in cycle ${i}"
    stop_daemon "${daemon_pid}" || true
    exit 1
  fi

  if ! stop_daemon "${daemon_pid}"; then
    echo "ERROR: cycle ${i} did not stop gracefully"
    exit 1
  fi

  sleep 2
  assert_no_runtime_orphans "after cycle ${i}"

  if awk -v re="${forbidden_pattern}" 'tolower($0) ~ tolower(re) {found=1} END {exit found ? 0 : 1}' "${log_file}"; then
    echo "ERROR: forbidden signature found in cycle ${i} log"
    awk -v re="${forbidden_pattern}" 'tolower($0) ~ tolower(re) {print NR ":" $0}' "${log_file}" || true
    exit 1
  fi

  echo "    cycle ${i}/${cycles} passed"
done

echo "==> [PR6 local gate] crash injection"
crash_log="/tmp/pr5-local-crash.log"
./build/edgelet daemon >"${crash_log}" 2>&1 &
daemon_pid=$!

if ! wait_until_ready "${daemon_pid}"; then
  echo "ERROR: daemon not ready before crash injection"
  stop_daemon "${daemon_pid}" || true
  exit 1
fi

if ! wait_dns_probe_ready; then
  echo "ERROR: DNS probe workloads not ready before crash injection"
  stop_daemon "${daemon_pid}" || true
  exit 1
fi

if ! run_dns_chaos_probe_burst 8; then
  echo "ERROR: DNS probe burst failed before crash injection"
  stop_daemon "${daemon_pid}" || true
  exit 1
fi

child_pid="$(pgrep -P "${daemon_pid}" -f -- "--edgelet-containerd-child" | head -n1 || true)"
if [[ -z "${child_pid}" ]]; then
  echo "ERROR: could not locate runtime child for crash injection"
  stop_daemon "${daemon_pid}" || true
  exit 1
fi

kill -KILL "${child_pid}" >/dev/null 2>&1 || true
deadline=$((SECONDS + crash_recovery_timeout_seconds))
while (( SECONDS < deadline )); do
  if ! kill -0 "${daemon_pid}" 2>/dev/null; then
    break
  fi
  sleep 1
done

if kill -0 "${daemon_pid}" 2>/dev/null; then
  echo "ERROR: daemon did not terminate after runtime crash within ${crash_recovery_timeout_seconds}s"
  stop_daemon "${daemon_pid}" || true
  exit 1
fi

assert_no_runtime_orphans "after crash injection"

post_crash_log="/tmp/pr5-local-post-crash.log"
./build/edgelet daemon >"${post_crash_log}" 2>&1 &
post_pid=$!
if ! wait_until_ready "${post_pid}"; then
  echo "ERROR: daemon not ready after crash-restart recovery"
  stop_daemon "${post_pid}" || true
  exit 1
fi
if ! wait_dns_probe_ready || ! run_dns_chaos_probe_burst 6; then
  echo "ERROR: DNS did not converge after crash-restart recovery"
  stop_daemon "${post_pid}" || true
  exit 1
fi
stop_daemon "${post_pid}" || true
assert_no_runtime_orphans "after post-crash restart validation"

if awk -v re="${forbidden_pattern}" 'tolower($0) ~ tolower(re) {found=1} END {exit found ? 0 : 1}' "${crash_log}"; then
  echo "ERROR: forbidden signature found in crash injection log"
  awk -v re="${forbidden_pattern}" 'tolower($0) ~ tolower(re) {print NR ":" $0}' "${crash_log}" || true
  exit 1
fi

echo "==> [PR6 local gate] passed"
