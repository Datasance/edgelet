#!/bin/bash
# Build script for ioFog Agent

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "Building ioFog Agent..."
make build

echo "Build complete!"
echo "Binaries:"
ls -lh build/
