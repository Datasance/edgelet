#!/usr/bin/env bash
# test/init/lib/embed-runtime.sh — Alpine/BusyBox helpers for embedded engine IT.
# Source from test/init/*-smoke.sh — do not execute directly.

# shellcheck shell=bash

# shellcheck source=test/init/lib/openrc-split-gate.sh
_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${_SCRIPT_DIR}/openrc-split-gate.sh"

# Remote bash snippet: reap stray --edgelet-containerd-child (BusyBox-safe pgrep).
# Optional STOP_OPENRC=1 runs rc-service edgelet stop first (may take up to STOP_TIMEOUT_SEC).
EMBED_RUNTIME_CLEANUP_SNIPPET='set -e
_stop_timeout="${STOP_TIMEOUT_SEC:-120}"
if [ "${STOP_OPENRC:-0}" = 1 ]; then
  rc-service edgelet stop &
  _spid=$!
  _elapsed=0
  while kill -0 "${_spid}" 2>/dev/null; do
    if [ "${_elapsed}" -ge "${_stop_timeout}" ]; then
      kill -9 "${_spid}" 2>/dev/null || true
      break
    fi
    sleep 2
    _elapsed=$(( _elapsed + 2 ))
  done
  wait "${_spid}" 2>/dev/null || true
fi
for _round in 1 2 3; do
  _pids=$(pgrep -f edgelet-containerd-child 2>/dev/null || true)
  [ -z "${_pids}" ] && break
  for _p in ${_pids}; do kill -TERM "${_p}" 2>/dev/null || true; done
  sleep 2
done
_pids=$(pgrep -f edgelet-containerd-child 2>/dev/null || true)
for _p in ${_pids}; do kill -KILL "${_p}" 2>/dev/null || true; done
_pids=$(pgrep -f "[e]dgelet daemon" 2>/dev/null || true)
for _p in ${_pids}; do kill -TERM "${_p}" 2>/dev/null || true; done
sleep 1
_pids=$(pgrep -f "[e]dgelet daemon" 2>/dev/null || true)
for _p in ${_pids}; do kill -KILL "${_p}" 2>/dev/null || true; done
rm -rf /run/edgelet /var/lib/edgelet-containerd/state
mkdir -p /run/edgelet /var/lib/edgelet-containerd/state
chmod 755 /run/edgelet /var/lib/edgelet-containerd/state'

# Remote bash snippet: wait for EdgeletAPI + embedded engine readiness.
EMBED_RUNTIME_WAIT_API_SNIPPET='set -e
_elapsed=0
_timeout="${API_WAIT_SEC:-180}"
while [ "${_elapsed}" -lt "${_timeout}" ]; do
  if { [ -S /run/edgelet/edgelet.sock ] || [ -S /var/run/edgelet/edgelet.sock ]; } && \
     { [ -S /run/edgelet/containerd.sock ] || [ -S /var/run/edgelet/containerd.sock ]; } && \
     edgelet system status 2>/dev/null | grep -q edgeletDaemon && \
     edgelet system status 2>/dev/null | grep -q 'runtime.engineReady: true'; then
    exit 0
  fi
  sleep 2
  _elapsed=$(( _elapsed + 2 ))
done
echo "edgelet API/runtime not ready after ${_timeout}s" >&2
pgrep -af "edgelet|containerd-child" 2>/dev/null || true
tail -n 60 /var/log/edgelet/edgelet.0.log 2>/dev/null || true
tail -n 40 /var/log/edgelet/daemon.log 2>/dev/null || true
exit 1'

# Remote bash snippet: assert fat edgelet is statically linked (file(1) or readelf fallback).
EMBED_RUNTIME_ASSERT_STATIC_SNIPPET='set -e
fat=$(readlink -f /var/lib/edgelet/data/current/bin/edgelet)
test -x "${fat}"
if command -v file >/dev/null 2>&1; then
  file -b "${fat}" | grep -q "statically linked"
elif command -v readelf >/dev/null 2>&1; then
  ! readelf -l "${fat}" 2>/dev/null | grep -q "INTERP"
else
  echo "need file or readelf to verify static fat edgelet" >&2
  exit 1
fi'

# Remote bash snippet: bounded OpenRC stop (edgelet-shutdown; no SSD --retry hang).
# Control-plane only — do not kill edgelet-containerd-child (Plan 11 split data plane).
EMBED_RUNTIME_OPENRC_STOP_SNIPPET='set -e
_stop_timeout="${STOP_TIMEOUT_SEC:-120}"
rc-service edgelet stop &
_spid=$!
_elapsed=0
while kill -0 "${_spid}" 2>/dev/null; do
  if [ "${_elapsed}" -ge "${_stop_timeout}" ]; then
    kill -9 "${_spid}" 2>/dev/null || true
    break
  fi
  sleep 2
  _elapsed=$(( _elapsed + 2 ))
done
wait "${_spid}" 2>/dev/null || true'

# Remote bash snippet: OpenRC data-plane restart (need edgelet-containerd restarts edgelet).
# Never chain rc-service edgelet restart after this.
EMBED_RUNTIME_OPENRC_RESTART_DATAPLANE_HEAD="${OPENRC_RESTART_DATAPLANE_SNIPPET}"
EMBED_RUNTIME_OPENRC_RESTART_DATAPLANE_SNIPPET="${EMBED_RUNTIME_OPENRC_RESTART_DATAPLANE_HEAD}
API_WAIT_SEC=\"\${API_WAIT_SEC:-180}\"
${OPENRC_WAIT_SPLIT_READY_SNIPPET}"

# Remote bash snippet: poll MS running after OpenRC restart (API + reconcile).
# Env: MS_NAME, MS_WAIT_SEC (optional).
EMBED_RUNTIME_OPENRC_WAIT_MS_SNIPPET="${OPENRC_WAIT_MS_RUNNING_SNIPPET}"

# Remote bash snippet: OpenRC restart (stop + start + API wait).
# Control-plane only — containerd stays up.
EMBED_RUNTIME_RESTART_SNIPPET='set -e
STOP_TIMEOUT_SEC="${STOP_TIMEOUT_SEC:-180}"
'"${EMBED_RUNTIME_OPENRC_STOP_SNIPPET}"'
rc-service edgelet start
API_WAIT_SEC="${API_WAIT_SEC:-180}"
'"${EMBED_RUNTIME_WAIT_API_SNIPPET}"''
