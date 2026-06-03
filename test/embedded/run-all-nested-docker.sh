#!/usr/bin/env bash
# Plan 11-7 Step 6 — build edgelet-linux image, then nested Docker smokes on the Mac host.
#
# Usage:
#   ./test/embedded/run-all-nested-docker.sh
#   ./test/embedded/run-all-nested-docker.sh --skip-build
#   ./test/embedded/run-all-nested-docker.sh --image=edgelet-linux:local --arch=arm64
#   ./test/embedded/run-all-nested-docker.sh --case=deploy
#   ./test/run-all.sh --suite=nested-docker
#
# Requires: Docker (privileged containers), jq, git (optional, for GIT_COMMIT build-arg).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/lima/lib/log.sh
source "${REPO_ROOT}/test/lima/lib/log.sh"
# shellcheck source=test/lima/lib/arch.sh
source "${REPO_ROOT}/test/lima/lib/arch.sh"

SKIP_BUILD=false
CASE="all"
TARGET_ARCH="$(lima_target_arch)"
IMAGE="${EDGELET_IMAGE:-edgelet-linux:local}"
VERSION="${EDGELET_VERSION:-dev}"

for arg in "$@"; do
    case "${arg}" in
        --skip-build) SKIP_BUILD=true ;;
        --arch=*)     TARGET_ARCH="${arg#*=}" ;;
        --image=*)    IMAGE="${arg#*=}" ;;
        --case=*)     CASE="${arg#*=}" ;;
        -h|--help)
            cat <<EOF
Usage: $0 [options]

Builds the scratch edgelet-linux image (Dockerfile.edgelet-linux), then runs nested
host Docker smokes. Not part of the Lima IT matrix.

Options:
  --skip-build       Use existing local image (--image / EDGELET_IMAGE)
  --arch=ARCH        linux platform arch: arm64 | amd64 (default: host)
  --image=NAME       Docker tag to build and test (default: edgelet-linux:local)
  --case=NAME        deploy | engine-switch | all (default: all)

Environment:
  EDGELET_IMAGE      Same as --image
  EDGELET_VERSION    build-arg VERSION (default: dev)
EOF
            exit 0
            ;;
        *) die "Unknown option: ${arg} (see --help)" ;;
    esac
done

command -v docker >/dev/null || die "docker CLI required"
command -v jq >/dev/null || die "jq required for nested smokes"

build_image() {
    local _commit _platform
    _commit="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
    _platform="linux/${TARGET_ARCH}"

    log_step "Building edgelet-linux image (${IMAGE}, platform=${_platform})"
    log_info "This runs the full embed pipeline inside Docker and may take 10–20+ minutes."

    cd "${REPO_ROOT}"

    if docker buildx version >/dev/null 2>&1; then
        docker buildx build \
            -f Dockerfile.edgelet-linux \
            --platform "${_platform}" \
            --build-arg "VERSION=${VERSION}" \
            --build-arg "GIT_COMMIT=${_commit}" \
            -t "${IMAGE}" \
            --load \
            .
    else
        log_warn "buildx not found — using docker build (host platform only)"
        docker build \
            -f Dockerfile.edgelet-linux \
            --build-arg "VERSION=${VERSION}" \
            --build-arg "GIT_COMMIT=${_commit}" \
            -t "${IMAGE}" \
            .
    fi

    log_ok "Image ready: ${IMAGE}"
}

if [[ "${SKIP_BUILD}" == false ]]; then
    build_image
else
    log_info "Skipping image build (--skip-build); using ${IMAGE}"
    if ! docker image inspect "${IMAGE}" >/dev/null 2>&1; then
        die "Image ${IMAGE} not found locally; remove --skip-build or pull/build first"
    fi
fi

export EDGELET_IMAGE="${IMAGE}"

FAIL=0
case "${CASE}" in
    deploy)
        "${SCRIPT_DIR}/container-deploy-smoke.sh" || FAIL=1
        ;;
    engine-switch)
        "${SCRIPT_DIR}/engine-switch-container.sh" || FAIL=1
        ;;
    all)
        "${SCRIPT_DIR}/container-deploy-smoke.sh" || FAIL=1
        [[ "${FAIL}" -eq 0 ]] && "${SCRIPT_DIR}/engine-switch-container.sh" || FAIL=1
        ;;
    *)
        die "Unknown --case=${CASE} (deploy | engine-switch | all)"
        ;;
esac

if [[ "${FAIL}" -ne 0 ]]; then
    die "nested-docker suite failed"
fi
log_success "nested-docker suite complete (image=${IMAGE})"
