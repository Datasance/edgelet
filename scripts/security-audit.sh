#!/usr/bin/env bash
# Edgelet security gate — delegates to Makefile targets (govulncheck + gosec).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "=== Edgelet security audit ==="
echo ""

make vulncheck
echo ""
make security-code

echo ""
echo "=== Security audit complete ==="
