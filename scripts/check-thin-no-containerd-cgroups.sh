#!/bin/bash
# Fail if the thin linux edgelet binary links containerd/cgroups (Plan 10 thin preflight split).
set -e

cd "$(dirname "$0")/.."

ARCH="${ARCH:-$(go env GOARCH)}"
BIN="build/edgelet-linux-${ARCH}"
if [ ! -f "${BIN}" ]; then
  echo "ERROR: missing ${BIN}; run scripts/build-edgelet first" >&2
  exit 1
fi

if go version -m "${BIN}" 2>/dev/null | grep -q 'github.com/containerd/cgroups'; then
  echo "ERROR: thin binary ${BIN} must not link github.com/containerd/cgroups (use DetectPreflight on !cgo)" >&2
  go version -m "${BIN}" | grep 'containerd/cgroups' || true
  exit 1
fi

echo "check-thin-no-containerd-cgroups: OK (${BIN})"
