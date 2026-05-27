#!/usr/bin/env bash
# Package edgelet release tarballs per RFC R20 (no musl suffix).
# Run from repo root: ./scripts/release-tarballs.sh [VERSION]
# Requires build/edgelet-linux-<arch> from make build-all-archs or scripts/build-edgelet.

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT="${ROOT}/dist"
mkdir -p "$OUT"

pack_linux() {
  _arch="$1"
  _bin="${ROOT}/build/edgelet-linux-${_arch}"
  if [ ! -f "$_bin" ]; then
    echo "skip (missing): $_bin" >&2
    return 0
  fi
  _name="edgelet-linux-${_arch}"
  _staging="${OUT}/staging-${_name}"
  rm -rf "$_staging"
  mkdir -p "$_staging"
  cp "$_bin" "$_staging/edgelet"
  if [ -f "${ROOT}/packaging/edgelet/etc/edgelet/config_new.yaml" ]; then
    cp "${ROOT}/packaging/edgelet/etc/edgelet/config_new.yaml" "$_staging/config.yaml.sample"
  elif [ -f "${ROOT}/packaging/edgelet/etc/edgelet/config_full.yaml" ]; then
    cp "${ROOT}/packaging/edgelet/etc/edgelet/config_full.yaml" "$_staging/config.yaml.sample"
  fi
  _tgz="${OUT}/${_name}.tar.gz"
  ( cd "$_staging" && tar -czf "$_tgz" . )
  rm -rf "$_staging"
  echo "Wrote ${_tgz}"

  _versioned="${OUT}/edgelet-${VERSION}-linux-${_arch}.tar.gz"
  cp "$_tgz" "$_versioned"
  echo "Wrote ${_versioned}"
}

pack_desktop() {
  _os="$1"
  _arch="$2"
  _bin="${ROOT}/build/edgelet-${_os}-${_arch}"
  if [ "${_os}" = windows ]; then
    _bin="${_bin}.exe"
  fi
  [ -f "$_bin" ] || return 0
  _name="edgelet-${_os}-${_arch}"
  _staging="${OUT}/staging-${_name}"
  rm -rf "$_staging"
  mkdir -p "$_staging"
  cp "$_bin" "$_staging/edgelet"
  _tgz="${OUT}/${_name}.tar.gz"
  ( cd "$_staging" && tar -czf "$_tgz" . )
  rm -rf "$_staging"
  echo "Wrote ${_tgz}"
}

# Linux matrix (unified — one binary per arch)
for _arch in amd64 arm64 arm riscv64; do
  pack_linux "${_arch}"
done

# Desktop monolithic artifacts (no -lite suffix)
for _pair in "darwin-amd64" "darwin-arm64" "windows-amd64"; do
  _os="${_pair%-*}"
  _arch="${_pair#*-}"
  pack_desktop "${_os}" "${_arch}"
done

( cd "$OUT" && sha256sum ./*.tar.gz 2>/dev/null > SHA256SUMS || true )

echo "Checksums: ${OUT}/SHA256SUMS"
