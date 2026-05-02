#!/usr/bin/env bash
# test/embedded/build.sh
#
# Downloads all embedded binary dependencies and cross-compiles iofog-agentd
# for the Linux target that will run inside the Lima VM.
#
# Outputs:
#   agent-go/build/iofog-agent-linux-<arch>
#   agent-go/build/iofog-agentd-linux-<arch>
#
# Usage:
#   ./test/embedded/build.sh [--arch=arm64|amd64]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
source "${SCRIPT_DIR}/lib/log.sh"

###############################################################################
# Detect target architecture
###############################################################################
HOST_ARCH="$(native_arch)"
case "${HOST_ARCH}" in
    arm64|aarch64) DEFAULT_ARCH="arm64" ;;
    x86_64)        DEFAULT_ARCH="amd64" ;;
    *)             DEFAULT_ARCH="amd64" ;;
esac

TARGET_ARCH="${DEFAULT_ARCH}"
for arg in "$@"; do
    case "${arg}" in
        --arch=*) TARGET_ARCH="${arg#*=}" ;;
    esac
done

log_step "Building iofog-agentd for linux/${TARGET_ARCH}"
log_info "Repository root: ${REPO_ROOT}"
log_info "Target arch: ${TARGET_ARCH}"

###############################################################################
# Step 1: Download embedded deps
###############################################################################
log_step "Downloading embedded binary dependencies"
cd "${REPO_ROOT}"

DEPS_SCRIPT="./build/download-deps.sh"
if [[ ! -x "${DEPS_SCRIPT}" ]]; then
    chmod +x "${DEPS_SCRIPT}"
fi

"${DEPS_SCRIPT}" --arch="${TARGET_ARCH}"
log_ok "Embedded dependencies downloaded"

###############################################################################
# Step 2: Select cross-compiler
###############################################################################
select_cc() {
    local arch="$1"
    # Prefer messense toolchains, fall back to Ubuntu cross-compilers
    for candidate in \
        "${arch}-unknown-linux-gnu-gcc" \
        "${arch}-linux-gnu-gcc" \
        "${arch}-linux-musl-gcc"; do
        if command -v "${candidate}" &>/dev/null; then
            echo "${candidate}"
            return
        fi
    done
    # Special case arm64 alias
    if [[ "${arch}" == "arm64" ]]; then
        for candidate in \
            "aarch64-unknown-linux-gnu-gcc" \
            "aarch64-linux-gnu-gcc" \
            "aarch64-linux-musl-gcc"; do
            if command -v "${candidate}" &>/dev/null; then
                echo "${candidate}"
                return
            fi
        done
    fi
    echo ""
}

###############################################################################
# Version / ldflags (full flavor; embedded tests require iofog engine)
###############################################################################
VERSION="$(cd "${REPO_ROOT}" && git describe --tags --always --dirty 2>/dev/null || echo dev)"
BUILD_TIME="$(date -u '+%Y-%m-%d_%H:%M:%S')"
GIT_COMMIT="$(cd "${REPO_ROOT}" && git rev-parse --short HEAD 2>/dev/null || echo unknown)"
LDFLAGS_FULL="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT} \
-X github.com/eclipse-iofog/agent/internal/buildmeta.Flavor=full"

###############################################################################
# Step 3: Build CLI (CGO not required)
###############################################################################
log_step "Building iofog-agent CLI (full flavor metadata)"
mkdir -p "${REPO_ROOT}/build"

CGO_ENABLED=0 GOOS=linux GOARCH="${TARGET_ARCH}" \
    go build \
    -trimpath \
    -ldflags "${LDFLAGS_FULL}" \
    -o "${REPO_ROOT}/build/iofog-agent-linux-${TARGET_ARCH}-full" \
    "${REPO_ROOT}/cmd/iofog-agent"

log_ok "CLI binary: build/iofog-agent-linux-${TARGET_ARCH}-full"

###############################################################################
# Step 4: Build daemon (CGO=1, needs cross-compiler)
###############################################################################
log_step "Building iofog-agentd daemon (embedded containerd, CGO=1, full flavor)"

# Resolve cross-compiler — map Go arch names to GNU triplet prefixes
GOARCH_FOR_CC="${TARGET_ARCH}"
[[ "${TARGET_ARCH}" == "arm64" ]] && GOARCH_FOR_CC="aarch64"
[[ "${TARGET_ARCH}" == "amd64" ]] && GOARCH_FOR_CC="x86_64"

CC="$(select_cc "${GOARCH_FOR_CC}")"
if [[ -z "${CC}" ]]; then
    log_warn "No cross-compiler found for ${TARGET_ARCH}."
    log_warn "Install with: brew install messense/macos-cross-toolchains/${GOARCH_FOR_CC}-unknown-linux-gnu"
    log_warn "Falling back to native compiler (only works if host=Linux)"
    CC="gcc"
fi
log_info "Using C compiler: ${CC}"

CGO_ENABLED=1 GOOS=linux GOARCH="${TARGET_ARCH}" CC="${CC}" \
    go build \
    -trimpath \
    -ldflags "${LDFLAGS_FULL} -extldflags '-static'" \
    -tags "cgo osusergo netgo" \
    -o "${REPO_ROOT}/build/iofog-agentd-linux-${TARGET_ARCH}-full" \
    "${REPO_ROOT}/cmd/iofog-agentd"

log_ok "Daemon binary: build/iofog-agentd-linux-${TARGET_ARCH}-full"

###############################################################################
# Done
###############################################################################
log_success "Build complete for linux/${TARGET_ARCH} (full flavor)"
echo ""
echo "  CLI:    build/iofog-agent-linux-${TARGET_ARCH}-full"
echo "  Daemon: build/iofog-agentd-linux-${TARGET_ARCH}-full"
