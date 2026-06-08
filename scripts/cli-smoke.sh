#!/usr/bin/env bash
# Optional manual smoke when edgeletd is running locally.
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

if ! edgelet system status -o json >/tmp/iofog-cli-smoke-status.json 2>/dev/null; then
  code=$?
  if [[ "$code" -eq 10 ]]; then
    echo "daemon unavailable (exit 10) — start edgeletd to run full smoke"
    exit 0
  fi
  exit "$code"
fi

jq -e '.iofogDaemon' /tmp/iofog-cli-smoke-status.json >/dev/null
edgelet ms ls -o json | jq -e '.items | type == "array"' >/dev/null

expect_fail edgelet status
expect_fail edgelet ms ps
expect_fail edgelet deploy apply -f /dev/null
expect_fail edgelet config set foo bar

echo "CLI smoke passed"
