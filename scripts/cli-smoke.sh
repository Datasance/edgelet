#!/usr/bin/env bash
# Optional manual smoke when iofog-agentd is running locally.
# CI uses Go tests in internal/cli/cmd/smoke_test.go instead.
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this script"
  exit 1
fi

expect_fail() {
  if "$@" >/dev/null 2>&1; then
    echo "expected failure for: $*"
    exit 1
  fi
}

if ! iofog-agent system status -o json >/tmp/iofog-cli-smoke-status.json 2>/dev/null; then
  code=$?
  if [[ "$code" -eq 10 ]]; then
    echo "daemon unavailable (exit 10) — start iofog-agentd to run full smoke"
    exit 0
  fi
  exit "$code"
fi

jq -e '.iofogDaemon' /tmp/iofog-cli-smoke-status.json >/dev/null
iofog-agent ms ls -o json | jq -e '.items | type == "array"' >/dev/null

expect_fail iofog-agent status
expect_fail iofog-agent ms ps
expect_fail iofog-agent deploy apply -f /dev/null
expect_fail iofog-agent config set foo bar

echo "CLI smoke passed"
