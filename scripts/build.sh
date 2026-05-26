#!/bin/bash
# Build script for ioFog Agent (default: full flavor, same as `make build`)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

FLAVOR="${FLAVOR:-full}"
echo "Building Edgelet (FLAVOR=${FLAVOR})..."
make build-edgelet "FLAVOR=${FLAVOR}"

echo "Build complete!"
echo "Binaries:"
ls -lh build/
