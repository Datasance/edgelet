#!/usr/bin/env bash
# test/release/smoke-linux-riscv64.sh — release bar for edgelet-linux-riscv64 (Tier 1).
#
# Checks: daemon start, edgelet version, CRI socket (/run/edgelet/containerd.sock).
#
# Usage:
#   ./test/release/build-all.sh
#   ./test/release/smoke-linux-riscv64.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${SCRIPT_DIR}/lib/smoke-linux-arch.sh" riscv64 linux/riscv64
