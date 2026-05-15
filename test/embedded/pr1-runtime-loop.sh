#!/usr/bin/env bash
set -euo pipefail

cycles="${1:-10}"
start_timeout_seconds="${START_TIMEOUT_SECONDS:-25}"
graceful_stop_timeout_seconds="${GRACEFUL_STOP_TIMEOUT_SECONDS:-45}"
startup_settle_seconds="${STARTUP_SETTLE_SECONDS:-8}"

echo "==> PR1 runtime loop validation (cycles=${cycles})"

mkdir -p /etc/iofog-agent
cp "packaging/iofog-agent/etc/iofog-agent/config_full.yaml" "/etc/iofog-agent/config.yaml"
cp "packaging/iofog-agent/etc/iofog-agent/cert_new.crt" "/etc/iofog-agent/cert.crt"

echo "==> Building full daemon (embedded containerd child-process flavor)"
make build-daemon-full

if [[ ! -x "./build/iofog-agentd" ]]; then
  echo "ERROR: build/iofog-agentd not found after build"
  exit 1
fi

success_cycles=0

for i in $(seq 1 "${cycles}"); do
  log_file="/tmp/pr1-runtime-loop-${i}.log"
  echo "==> Cycle ${i}/${cycles}: starting daemon"

  "./build/iofog-agentd" start >"${log_file}" 2>&1 &
  daemon_pid=$!
  echo "    daemon pid=${daemon_pid}, log=${log_file}"

  child_seen=0
  deadline=$((SECONDS + start_timeout_seconds))
  while (( SECONDS < deadline )); do
    if ! kill -0 "${daemon_pid}" 2>/dev/null; then
      break
    fi
    if pgrep -P "${daemon_pid}" -f -- "--iofog-containerd-child" >/dev/null 2>&1; then
      child_seen=1
      break
    fi
    sleep 1
  done

  if [[ "${child_seen}" -ne 1 ]]; then
    echo "ERROR: cycle ${i} did not observe child process before timeout"
    if kill -0 "${daemon_pid}" 2>/dev/null; then
      kill -TERM "${daemon_pid}" || true
      timeout "${graceful_stop_timeout_seconds}" bash -lc "wait ${daemon_pid}" || true
    fi
    exit 1
  fi

  # Give the daemon time to finish startup and install full signal handling.
  sleep "${startup_settle_seconds}"

  # PR6 lightweight smoke: when LocalAPI is reachable, DNS status/metrics should be visible.
  if ./build/iofog-agent system status >/tmp/pr1-runtime-status-${i}.txt 2>/dev/null; then
    if ! grep -q "dnsHealth" "/tmp/pr1-runtime-status-${i}.txt"; then
      echo "ERROR: cycle ${i} missing dnsHealth in system status"
      kill -TERM "${daemon_pid}" || true
      exit 1
    fi
    if command -v curl >/dev/null 2>&1; then
      metrics="$(curl -ksSf https://127.0.0.1:54321/metrics 2>/dev/null || true)"
      if [[ -n "${metrics}" ]]; then
        if ! echo "${metrics}" | grep -q "iofog_dns_queries_total"; then
          echo "ERROR: cycle ${i} missing iofog_dns_queries_total in metrics"
          kill -TERM "${daemon_pid}" || true
          exit 1
        fi
      fi
    fi
  fi

  echo "    child process observed, stopping daemon"
  kill -TERM "${daemon_pid}"

  stop_deadline=$((SECONDS + graceful_stop_timeout_seconds))
  while (( SECONDS < stop_deadline )); do
    if ! kill -0 "${daemon_pid}" 2>/dev/null; then
      break
    fi
    sleep 1
  done
  if kill -0 "${daemon_pid}" 2>/dev/null; then
    echo "ERROR: cycle ${i} daemon did not stop within timeout"
    kill -KILL "${daemon_pid}" || true
    exit 1
  fi

  # Allow child reaping to complete.
  sleep 2

  if awk 'tolower($0) ~ /text file busy|etxtbsy/ {found=1} END {exit found ? 0 : 1}' "${log_file}"; then
    echo "ERROR: cycle ${i} log contains ETXTBSY signature"
    awk 'tolower($0) ~ /text file busy|etxtbsy/ {print NR ":" $0}' "${log_file}" || true
    exit 1
  fi

  if pgrep -af -- "--iofog-containerd-child" >/tmp/pr1-orphan-processes.txt 2>&1; then
    echo "ERROR: orphan containerd child detected after cycle ${i}"
    cat /tmp/pr1-orphan-processes.txt
    exit 1
  fi

  success_cycles=$((success_cycles + 1))
  echo "    cycle ${i} passed"
done

echo "==> Validation passed: ${success_cycles}/${cycles} cycles without orphan child process"
