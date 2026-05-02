#!/usr/bin/env sh
# Package per-flavor tarballs and SHA256SUMS-lite / SHA256SUMS-full from make build outputs.
# Run from repo root: ./scripts/release-tarballs.sh [VERSION]
# Requires: make build-linux-amd64 (or full build-all-archs) already run.

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT="${ROOT}/build/release"
mkdir -p "$OUT"

# Map: makefile artifact suffix -> tarball arch label (must match install.sh)
# build/iofog-agent-linux-amd64-lite -> amd64
pack_one() {
	_suffix="$1"   # e.g. amd64, amd64-musl, arm64, arm64-musl, arm, riscv64
	_label="$2"  # same for tarball name

	for _fl in lite full; do
		_cli="${ROOT}/build/iofog-agent-linux-${_suffix}-${_fl}"
		_daemon="${ROOT}/build/iofog-agentd-linux-${_suffix}-${_fl}"
		if [ ! -f "$_cli" ] || [ ! -f "$_daemon" ]; then
			echo "skip (missing): $_cli / $_daemon" >&2
			continue
		fi
		_name="iofog-agent-${VERSION}-linux-${_label}-${_fl}"
		_staging="${OUT}/staging-${_name}"
		rm -rf "$_staging"
		mkdir -p "$_staging"
		cp "$_cli" "$_staging/iofog-agent"
		cp "$_daemon" "$_staging/iofog-agentd"
		if [ -f "${ROOT}/packaging/iofog-agent/etc/iofog-agent/config_new.yaml" ]; then
			cp "${ROOT}/packaging/iofog-agent/etc/iofog-agent/config_new.yaml" "$_staging/config.yaml.sample"
		fi
		_tgz="${OUT}/${_name}.tar.gz"
		( cd "$_staging" && tar -czf "$_tgz" . )
		rm -rf "$_staging"
		echo "Wrote ${_tgz}"
	done
}

# Default: all glibc + musl variants produced by Makefile
pack_one "amd64" "amd64"
pack_one "amd64-musl" "amd64-musl"
pack_one "arm64" "arm64"
pack_one "arm64-musl" "arm64-musl"
pack_one "arm" "arm"
pack_one "riscv64" "riscv64"

( cd "$OUT" && sha256sum ./*.tar.gz 2>/dev/null | grep -E '\-lite\.tar\.gz$' > SHA256SUMS-lite || true )
( cd "$OUT" && sha256sum ./*.tar.gz 2>/dev/null | grep -E '\-full\.tar\.gz$' > SHA256SUMS-full || true )

echo "Checksums: ${OUT}/SHA256SUMS-lite ${OUT}/SHA256SUMS-full"
