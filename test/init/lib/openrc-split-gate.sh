#!/usr/bin/env bash
# test/init/lib/openrc-split-gate.sh — OpenRC runtime split wait gates. Source only.

# shellcheck shell=bash

# Remote snippet: wait for containerd.sock (+ ctr version when available).
OPENRC_WAIT_CONTAINERD_SNIPPET='set -e
_elapsed=0
_timeout="${CONTAINERD_WAIT_SEC:-120}"
while [ "${_elapsed}" -lt "${_timeout}" ]; do
  if [ -S /run/edgelet/containerd.sock ] || [ -S /var/run/edgelet/containerd.sock ]; then
    _ctr=""
    if [ -x /var/lib/edgelet/data/current/bin/ctr ]; then
      _ctr=/var/lib/edgelet/data/current/bin/ctr
    elif command -v ctr >/dev/null 2>&1; then
      _ctr=ctr
    fi
    if [ -z "${_ctr}" ] || "${_ctr}" --address /run/edgelet/containerd.sock version >/dev/null 2>&1; then
      exit 0
    fi
  fi
  sleep 2
  _elapsed=$(( _elapsed + 2 ))
done
echo "containerd socket not ready after ${_timeout}s" >&2
pgrep -af "edgelet|containerd-child" 2>/dev/null || true
ls -la /run/edgelet/ 2>/dev/null || true
exit 1'

# Remote snippet: restart data plane only; need edgelet-containerd restarts edgelet.
# Do NOT rc-service edgelet restart afterwards.
OPENRC_RESTART_DATAPLANE_SNIPPET='set -e
rc-service edgelet-containerd restart
CONTAINERD_WAIT_SEC="${CONTAINERD_WAIT_SEC:-120}"
'
OPENRC_RESTART_DATAPLANE_SNIPPET+="${OPENRC_WAIT_CONTAINERD_SNIPPET}"

# Remote snippet: full split readiness (both sockets + runtime.engineReady).
OPENRC_WAIT_SPLIT_READY_SNIPPET='set -e
_elapsed=0
_timeout="${API_WAIT_SEC:-180}"
while [ "${_elapsed}" -lt "${_timeout}" ]; do
  if { [ -S /run/edgelet/containerd.sock ] || [ -S /var/run/edgelet/containerd.sock ]; } && \
     { [ -S /run/edgelet/edgelet.sock ] || [ -S /var/run/edgelet/edgelet.sock ]; } && \
     edgelet system status 2>/dev/null | grep -q edgeletDaemon && \
     edgelet system status -o json 2>/dev/null | jq -e ".[\"runtime.engineReady\"] == \"true\"" >/dev/null; then
    exit 0
  fi
  sleep 2
  _elapsed=$(( _elapsed + 2 ))
done
echo "openrc split not ready after ${_timeout}s" >&2
pgrep -af "edgelet|containerd-child" 2>/dev/null || true
rc-status 2>/dev/null | grep -E "edgelet" || true
tail -n 60 /var/log/edgelet/edgelet.0.log 2>/dev/null || true
tail -n 40 /var/log/edgelet/daemon.log 2>/dev/null || true
tail -n 40 /var/log/edgelet/containerd.log 2>/dev/null || true
exit 1'

# Remote snippet: poll until EdgeletAPI accepts requests, engineReady, and MS is running.
# Env: MS_NAME (required), MS_WAIT_SEC (default 240).
OPENRC_WAIT_MS_RUNNING_SNIPPET='set -e
_ms_name="${MS_NAME:?MS_NAME required}"
_elapsed=0
_timeout="${MS_WAIT_SEC:-240}"
while [ "${_elapsed}" -lt "${_timeout}" ]; do
  if edgelet system status -o json 2>/dev/null \
       | jq -e ".[\"runtime.engineReady\"] == \"true\"" >/dev/null \
     && edgelet ms ls 2>/dev/null | grep -F "${_ms_name}" | grep -qi running; then
    exit 0
  fi
  sleep 2
  _elapsed=$(( _elapsed + 2 ))
done
echo "MS ${_ms_name} not running after ${_timeout}s (post-restart reconcile)" >&2
edgelet ms ls 2>&1 || true
edgelet system status -o json 2>/dev/null \
  | jq "{engineReady: .[\"runtime.engineReady\"]}" || true
pgrep -af "edgelet|containerd-child" 2>/dev/null || true
exit 1'
