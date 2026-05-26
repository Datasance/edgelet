#!/bin/bash
set -e

cd "$(dirname "$0")/.."

. ./scripts/version.sh

GO=${GO-go}
ARCH=${ARCH:-$("${GO}" env GOARCH)}

if [ "${DEBUG}" = 1 ]; then
  set -x
fi

MAX_BINARY_MB=55
MAX_BINARY_SIZE=$((MAX_BINARY_MB * 1024 * 1024))

CMD_NAME="build/edgelet-linux-${ARCH}-full"
if [ ! -f "${CMD_NAME}" ]; then
  CMD_NAME="build/edgelet"
fi

if [ ! -f "${CMD_NAME}" ]; then
  echo "ERROR: binary not found: ${CMD_NAME} (run scripts/build-edgelet or make build-edgelet-full first)" >&2
  exit 1
fi

if [ "$(uname -s)" = Darwin ]; then
  SIZE=$(stat -f '%z' "${CMD_NAME}")
else
  SIZE=$(stat -c '%s' "${CMD_NAME}")
fi

if [ -n "${DEBUG}" ]; then
  echo "DEBUG is set, ignoring binary size (${SIZE} bytes)"
  exit 0
fi

if [ "${SIZE}" -gt "${MAX_BINARY_SIZE}" ]; then
  echo "edgelet full binary ${CMD_NAME} size ${SIZE} exceeds max ${MAX_BINARY_SIZE} bytes (${MAX_BINARY_MB} MiB)" >&2
  exit 1
fi

echo "edgelet full binary ${CMD_NAME} size ${SIZE} is within ${MAX_BINARY_MB} MiB limit"
exit 0
