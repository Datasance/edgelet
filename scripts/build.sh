#!/bin/bash
# Build script for Edgelet (default: host OS build, same as `make build`)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "Building Edgelet..."
make build

echo "Build complete!"
echo "Binaries:"
ls -lh build/
