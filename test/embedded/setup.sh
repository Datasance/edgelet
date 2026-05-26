#!/usr/bin/env bash
# test/embedded/setup.sh
#
# Checks and installs all macOS prerequisites needed to build and test
# edgeletd with the embedded containerd engine inside a Lima VM.
#
# Requirements installed via Homebrew:
#   - lima          (lightweight Linux VM manager)
#   - jq            (JSON parsing)
#   - aarch64 / x86_64 cross-compilers  (CGO_ENABLED=1 cross-compile to Linux)
#
# Usage:
#   ./test/embedded/setup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

###############################################################################
# 1. Homebrew
###############################################################################
if ! command -v brew &>/dev/null; then
    die "Homebrew is not installed. Install it from https://brew.sh and re-run."
fi
log_info "Homebrew: OK ($(brew --version | head -1))"

###############################################################################
# 2. Lima
###############################################################################
if ! command -v limactl &>/dev/null; then
    log_info "Installing Lima..."
    brew install lima
else
    log_info "Lima: OK ($(limactl --version))"
fi

# Lima 0.14+ required for vzNAT (host-reachable VM IP with vmType: vz)
if command -v limactl &>/dev/null; then
    LIMA_VER="$(limactl --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | head -1)"
    if [[ -n "${LIMA_VER}" ]]; then
        LIMA_MAJOR="${LIMA_VER%%.*}"
        LIMA_MINOR="${LIMA_VER##*.}"
        if [[ "${LIMA_MAJOR}" -lt 0 ]] || { [[ "${LIMA_MAJOR}" -eq 0 ]] && [[ "${LIMA_MINOR}" -lt 14 ]]; }; then
            log_warn "Lima ${LIMA_VER} detected. vzNAT (in lima-ubuntu.yaml) requires Lima 0.14+. Upgrade: brew upgrade lima"
        fi
    fi
fi

###############################################################################
# 3. jq
###############################################################################
if ! command -v jq &>/dev/null; then
    log_info "Installing jq..."
    brew install jq
else
    log_info "jq: OK ($(jq --version))"
fi

###############################################################################
# 4. Cross-compiler selection (based on host arch)
###############################################################################
HOST_ARCH="$(native_arch)"

if [[ "${HOST_ARCH}" == "arm64" ]]; then
    # Building linux/arm64 binary on Apple Silicon — native cross-compile.
    TARGET_CC="aarch64-unknown-linux-gnu-gcc"
    BREW_CROSS="aarch64-unknown-linux-gnu"

    if ! command -v "${TARGET_CC}" &>/dev/null; then
        log_info "Installing aarch64 Linux cross-compiler..."
        # Try the musl-cross tap which provides aarch64-unknown-linux-gnu
        brew tap messense/macos-cross-toolchains 2>/dev/null || true
        brew install aarch64-unknown-linux-gnu 2>/dev/null || \
        brew install FiloSottile/musl-cross/musl-cross 2>/dev/null || \
        log_warn "Could not install ${BREW_CROSS} automatically." \
                 "Install manually: brew install messense/macos-cross-toolchains/aarch64-unknown-linux-gnu"
    else
        log_info "Cross-compiler ${TARGET_CC}: OK"
    fi

else
    # Intel Mac — cross-compile to linux/amd64 is easier (same arch as target).
    TARGET_CC="x86_64-linux-gnu-gcc"
    if ! command -v "${TARGET_CC}" &>/dev/null; then
        log_info "Installing x86_64 Linux cross-compiler..."
        brew tap messense/macos-cross-toolchains 2>/dev/null || true
        brew install x86_64-unknown-linux-gnu 2>/dev/null || \
        log_warn "Could not install cross-compiler automatically." \
                 "Install manually: brew install messense/macos-cross-toolchains/x86_64-unknown-linux-gnu"
    else
        log_info "Cross-compiler ${TARGET_CC}: OK"
    fi
fi

###############################################################################
# 5. Go
###############################################################################
if ! command -v go &>/dev/null; then
    die "Go is not installed. Install it from https://go.dev/dl/"
fi
log_info "Go: OK ($(go version))"

###############################################################################
# Done
###############################################################################
log_success "All prerequisites are installed. Run './test/embedded/run-all.sh' to start testing."
