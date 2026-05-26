#!/usr/bin/env bash
# Package edgelet release tarballs per RFC R20 (no musl suffix).
# Run from repo root: ./scripts/release-tarballs.sh [VERSION]
# Requires build/edgelet-linux-*-{lite,full} from make build-all-archs or scripts/build-edgelet.

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT="${ROOT}/dist"
mkdir -p "$OUT"

pack_one() {
  _os="$1"
  _arch="$2"

  for _fl in lite full; do
    _bin="${ROOT}/build/edgelet-${_os}-${_arch}-${_fl}"
    if [ ! -f "$_bin" ]; then
      echo "skip (missing): $_bin" >&2
      continue
    fi
    _name="edgelet-${_os}-${_arch}-${_fl}"
    _staging="${OUT}/staging-${_name}"
    rm -rf "$_staging"
    mkdir -p "$_staging"
    cp "$_bin" "$_staging/edgelet"
    if [ -f "${ROOT}/packaging/edgelet/etc/edgelet/config_new.yaml" ]; then
      cp "${ROOT}/packaging/edgelet/etc/edgelet/config_new.yaml" "$_staging/config.yaml.sample"
    elif [ -f "${ROOT}/packaging/edgelet/etc/edgelet/config_full.yaml" ] && [ "$_fl" = full ]; then
      cp "${ROOT}/packaging/edgelet/etc/edgelet/config_full.yaml" "$_staging/config.yaml.sample"
    elif [ -f "${ROOT}/packaging/edgelet/etc/edgelet/config_lite.yaml" ] && [ "$_fl" = lite ]; then
      cp "${ROOT}/packaging/edgelet/etc/edgelet/config_lite.yaml" "$_staging/config.yaml.sample"
    fi
    _tgz="${OUT}/${_name}.tar.gz"
    ( cd "$_staging" && tar -czf "$_tgz" . )
    rm -rf "$_staging"
    echo "Wrote ${_tgz}"

    _versioned="${OUT}/edgelet-${VERSION}-${_os}-${_arch}-${_fl}.tar.gz"
    cp "$_tgz" "$_versioned"
    echo "Wrote ${_versioned}"
  done
}

# Linux matrix (full + lite)
for _arch in amd64 arm64 arm riscv64; do
  pack_one linux "${_arch}"
done

# Desktop lite-only smoke artifacts
for _pair in "darwin-amd64" "darwin-arm64" "windows-amd64"; do
  _os="${_pair%-*}"
  _arch="${_pair#*-}"
  _bin="${ROOT}/build/edgelet-${_os}-${_arch}-lite"
  [ -f "$_bin" ] || continue
  _name="edgelet-${_os}-${_arch}-lite"
  _staging="${OUT}/staging-${_name}"
  rm -rf "$_staging"
  mkdir -p "$_staging"
  cp "$_bin" "$_staging/edgelet"
  _tgz="${OUT}/${_name}.tar.gz"
  ( cd "$_staging" && tar -czf "$_tgz" . )
  rm -rf "$_staging"
  echo "Wrote ${_tgz}"
done

( cd "$OUT" && sha256sum ./*-lite.tar.gz 2>/dev/null > SHA256SUMS-lite || true )
( cd "$OUT" && sha256sum ./*-full.tar.gz 2>/dev/null > SHA256SUMS-full || true )

echo "Checksums: ${OUT}/SHA256SUMS-lite ${OUT}/SHA256SUMS-full"
