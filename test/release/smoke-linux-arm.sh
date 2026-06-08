#!/usr/bin/env bash
# test/release/smoke-linux-arm.sh — release bar for edgelet-linux-arm (Tier 1).
#
# Checks: daemon start, edgelet version, CRI socket (/run/edgelet/containerd.sock).
#
# Usage:
#   ./test/release/build-all.sh
#   ./test/release/smoke-linux-arm.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${SCRIPT_DIR}/lib/smoke-linux-arch.sh" arm linux/arm/v7
