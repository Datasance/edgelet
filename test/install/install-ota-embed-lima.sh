#!/usr/bin/env bash
# test/install/install-ota-embed-lima.sh
#
# End-to-end OTA on Lima: published release v1.0.1 -> v1.0.2-rc.1 using the repo
# install.sh (embed-hash change + conditional edgelet-containerd restart).
#
# Flow:
#   1. Fresh install baseline release with current install.sh (embedded split)
#   2. Deploy two local microservices
#   3. install.sh --upgrade to target release
#   4. Assert data-plane restart, embed rotation, containerd health, MS reconcile
#   5. Same-version upgrade must not request data-plane restart
#
# Usage (macOS host with Lima):
#   ./test/install/install-ota-embed-lima.sh
#   ./test/install/install-ota-embed-lima.sh --vm-name=edgelet-ota-test --arch=arm64
#   ./test/install/install-ota-embed-lima.sh --from-version=v1.0.1 --to-version=v1.0.2-rc.1
#   ./test/install/install-ota-embed-lima.sh --skip-vm-start --skip-download
#   ./test/install/install-ota-embed-lima.sh --github-repo=Datasance/edgelet
#
# Requires: limactl, jq, curl, network for GitHub release downloads (unless --skip-download).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=test/lima/lib/log.sh
source "${REPO_ROOT}/test/lima/lib/log.sh"
# shellcheck source=test/lima/lib/arch.sh
source "${REPO_ROOT}/test/lima/lib/arch.sh"
# shellcheck source=test/lima/lib/remote.sh
source "${REPO_ROOT}/test/lima/lib/remote.sh"
# shellcheck source=test/lima/lib/split-gate.sh
source "${REPO_ROOT}/test/lima/lib/split-gate.sh"
# shellcheck source=test/init/lib/stage-install-bundle.sh
source "${REPO_ROOT}/test/init/lib/stage-install-bundle.sh"

VM_NAME="edgelet-ota-test"
TARGET_ARCH="$(lima_target_arch)"
FROM_VERSION="v1.0.1"
TO_VERSION="v1.0.2-rc.1"
GITHUB_REPO="${EDGELET_GITHUB_REPO:-eclipse-iofog/edgelet}"
SKIP_VM_START=false
SKIP_DOWNLOAD=false
CLEAN=true
LIMA_YAML="${REPO_ROOT}/test/embedded/lima-ubuntu.yaml"

CACHE_DIR="${REPO_ROOT}/.cache/ota-lima-bins"
STAGE_DIR="/tmp/edgelet-ota-lima-stage"
MS_A="ota-lima-ms-a"
MS_B="ota-lima-ms-b"
RECEIPT="/var/backups/edgelet/install-receipt"
BUNDLED_INSTALL="/usr/share/edgelet/install.sh"

for arg in "$@"; do
    case "${arg}" in
        --vm-name=*) VM_NAME="${arg#*=}" ;;
        --arch=*) TARGET_ARCH="${arg#*=}" ;;
        --from-version=*) FROM_VERSION="${arg#*=}" ;;
        --to-version=*) TO_VERSION="${arg#*=}" ;;
        --github-repo=*) GITHUB_REPO="${arg#*=}" ;;
        --skip-vm-start) SKIP_VM_START=true ;;
        --skip-download) SKIP_DOWNLOAD=true ;;
        --no-clean) CLEAN=false ;;
        --lima-yaml=*) LIMA_YAML="${arg#*=}" ;;
        -h|--help)
            sed -n '1,22p' "$0" | sed 's/^# \?//'
            exit 0
            ;;
        *) die "Unknown option: ${arg} (see --help)" ;;
    esac
done

if [[ "$(uname -s)" != "Darwin" ]]; then
    die "This test runs on macOS with Lima. For native Linux OTA smoke use test/install/install-upgrade-rollback.sh"
fi

command -v limactl >/dev/null || die "limactl not found — run test/embedded/setup.sh"
command -v jq >/dev/null || die "jq required"
command -v curl >/dev/null || die "curl required"

INSTALL_SH="${REPO_ROOT}/install.sh"
[[ -f "${INSTALL_SH}" ]] || die "Missing ${INSTALL_SH}"
grep -q 'should_restart_data_plane' "${INSTALL_SH}" \
    || die "install.sh missing embed OTA helpers — run make install-scripts and retry"

validate_install_bundle_sources "${REPO_ROOT}"

vm_r() {
    echo "$*" | limactl --tty=false shell "${VM_NAME}" -- sudo bash
}

release_bin_path() {
    local _ver="$1"
    echo "${CACHE_DIR}/edgelet-${_ver}-linux-${TARGET_ARCH}"
}

download_release_bin() {
    local _ver="$1" _dest="$2"
    local _url="https://github.com/${GITHUB_REPO}/releases/download/${_ver}/edgelet-linux-${TARGET_ARCH}"
    log_step "Downloading ${_ver} from ${_url}"
    mkdir -p "${CACHE_DIR}"
    curl -fsSL -o "${_dest}" "${_url}" || die "Failed to download ${_url}"
    chmod +x "${_dest}"
    log_ok "Cached ${_dest}"
}

ensure_release_bins() {
    local _from _to
    _from="$(release_bin_path "${FROM_VERSION}")"
    _to="$(release_bin_path "${TO_VERSION}")"
    if [[ "${SKIP_DOWNLOAD}" != true ]]; then
        download_release_bin "${FROM_VERSION}" "${_from}"
        download_release_bin "${TO_VERSION}" "${_to}"
    fi
    [[ -f "${_from}" ]] || die "Missing baseline binary ${_from} (drop --skip-download or fetch release)"
    [[ -f "${_to}" ]] || die "Missing upgrade binary ${_to} (drop --skip-download or fetch release)"
}

ensure_vm() {
    if [[ "${SKIP_VM_START}" == true ]]; then
        [[ "$(lima_status "${VM_NAME}")" == "Running" ]] \
            || die "VM ${VM_NAME} is not running (drop --skip-vm-start or start VM)"
        return 0
    fi
    "${REPO_ROOT}/test/embedded/vm-start.sh" \
        --vm-name="${VM_NAME}" \
        --lima-yaml="${LIMA_YAML}"
}

stage_bin_to_vm() {
    local _host_bin="$1" _vm_path="$2"
    local _ssh_config _ssh_host _tmpdir
    _ssh_config="$(lima_ssh_config "${VM_NAME}")"
    _ssh_host="$(lima_ssh_host "${VM_NAME}")"
    _tmpdir="$(mktemp -d)"
    cp "${_host_bin}" "${_tmpdir}/edgelet"
    chmod +x "${_tmpdir}/edgelet"
    ssh -F "${_ssh_config}" "${_ssh_host}" "mkdir -p '$(dirname "${_vm_path}")'"
    COPYFILE_DISABLE=1 tar -C "${_tmpdir}" -cf - edgelet \
        | ssh -F "${_ssh_config}" "${_ssh_host}" "tar -xf - -C '$(dirname "${_vm_path}")'"
    rm -rf "${_tmpdir}"
    vm_r "mv '$(dirname "${_vm_path}")/edgelet' '${_vm_path}' && chmod 755 '${_vm_path}'"
}

embed_hash_for_bin() {
    local _bin="$1" _hash
    _hash="$(vm_r "${_bin} version --verbose 2>/dev/null | sed -n 's/^  embed hash: //p' | head -1")"
    printf '%s' "${_hash}" | tr -d '\r\n'
}

wait_ms_name_running() {
    local _name="$1" _timeout="${2:-240}" _elapsed=0
    while (( _elapsed < _timeout )); do
        if vm_r "edgelet ms ls 2>/dev/null | grep -F '${_name}' | grep -Eiq 'running'"; then
            return 0
        fi
        sleep 5
        _elapsed=$(( _elapsed + 5 ))
    done
    vm_r "edgelet ms ls || true"
    return 1
}

wait_containerd_config_v4() {
    local _timeout="${1:-120}" _elapsed=0
    while (( _elapsed < _timeout )); do
        if vm_r "grep -q '^version = 4' /var/lib/edgelet-containerd/config.toml"; then
            return 0
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done
    vm_r "head -20 /var/lib/edgelet-containerd/config.toml || true"
    return 1
}

deploy_ota_workloads() {
    log_step "Deploying local microservices (${MS_A}, ${MS_B})"
    vm_r "cat >/tmp/${MS_A}.yaml <<'EOF'
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: ${MS_A}
spec:
  image: docker.io/library/alpine:3.19
  registry: 1
  container:
    hostNetworkMode: false
    commands: [/bin/sh, -lc, \"sleep 7200\"]
  schedule: 50
EOF
cat >/tmp/${MS_B}.yaml <<'EOF'
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: ${MS_B}
spec:
  image: docker.io/library/busybox:1.36
  registry: 1
  container:
    hostNetworkMode: false
    commands: [/bin/sh, -lc, \"sleep 7200\"]
  schedule: 50
EOF
edgelet deploy -f /tmp/${MS_A}.yaml
edgelet deploy -f /tmp/${MS_B}.yaml"

    wait_ms_name_running "${MS_A}" 300 || die "Timed out waiting for ${MS_A} RUNNING"
    wait_ms_name_running "${MS_B}" 300 || die "Timed out waiting for ${MS_B} RUNNING"
    log_ok "Both workloads RUNNING before upgrade"
}

fresh_install_baseline() {
    local _from_host _vm_bin="${STAGE_DIR}/edgelet-from"
    _from_host="$(release_bin_path "${FROM_VERSION}")"
    local _ssh_config _ssh_host

    log_step "Fresh install ${FROM_VERSION} (repo install.sh, containerEngine=edgelet)"
    vm_r "systemctl stop edgelet edgelet-containerd 2>/dev/null || true"
    if [[ "${CLEAN}" == true ]]; then
        vm_r "if [ -x /usr/share/edgelet/uninstall.sh ]; then
            sh /usr/share/edgelet/uninstall.sh --remove-data || true
        fi"
    fi

    _ssh_config="$(lima_ssh_config "${VM_NAME}")"
    _ssh_host="$(lima_ssh_host "${VM_NAME}")"
    stage_install_bundle_ssh "${_ssh_config}" "${_ssh_host}" "${STAGE_DIR}" "${REPO_ROOT}" "${_from_host}"

    vm_r "set -e
chmod +x ${STAGE_DIR}/install.sh ${STAGE_DIR}/edgelet
grep -q should_restart_data_plane ${STAGE_DIR}/install.sh
${STAGE_DIR}/install.sh \
    --bin-path=${STAGE_DIR}/edgelet \
    --version=${FROM_VERSION} \
    --arch=${TARGET_ARCH} \
    --container-engine=edgelet"

    lima_wait_split_units "${VM_NAME}" 180 \
        || die "Split units not active after baseline install"
    lima_wait_edgelet_api "${VM_NAME}" 180 \
        || die "Edgelet API not ready after baseline install"

    vm_r "grep -q should_restart_data_plane ${BUNDLED_INSTALL}" \
        || die "Bundled ${BUNDLED_INSTALL} missing embed OTA helpers"

    log_ok "Baseline ${FROM_VERSION} installed with updated install.sh"
}

run_embed_upgrade() {
    local _to_host _vm_bin="/var/tmp/edgelet-${TO_VERSION}"
    _to_host="$(release_bin_path "${TO_VERSION}")"

    log_step "Upgrade ${FROM_VERSION} -> ${TO_VERSION} via ${BUNDLED_INSTALL} --upgrade"
    stage_bin_to_vm "${_to_host}" "${_vm_bin}"

    local _upgrade_log
    _upgrade_log="$(mktemp)"
    # shellcheck disable=SC2064
    trap "rm -f '${_upgrade_log}'" RETURN

    vm_r "set -e
${BUNDLED_INSTALL} \
    --upgrade \
    --bin-path=${_vm_bin} \
    --version=${TO_VERSION} \
    --arch=${TARGET_ARCH}" 2>&1 | tee "${_upgrade_log}"

    grep -q "data-plane restart required" "${_upgrade_log}" \
        || die "Upgrade log missing 'data-plane restart required' (embed hash gate)"
    grep -q "restarting edgelet-containerd" "${_upgrade_log}" \
        || die "Upgrade log missing edgelet-containerd restart message"

    log_ok "Upgrade script requested data-plane restart"
}

assert_post_upgrade() {
    local _h1="$1" _h2="$2" _old_ctd_pid="$3"

    log_step "Post-upgrade assertions"

    assert_ok "install receipt at ${TO_VERSION}" \
        vm_r "grep -q '^installed_version=${TO_VERSION}\$' ${RECEIPT}"

    assert_ok "previous-release records ${FROM_VERSION}" \
        vm_r "grep -q '^previous_version=${FROM_VERSION}\$' /var/backups/edgelet/previous-release"

    assert_ok "embed hash rotated on disk (current != baseline)" \
        vm_r "test \"\$(basename \"\$(readlink -f /var/lib/edgelet/data/current)\")\" = \"${_h2}\""

    assert_ok "data/previous symlink exists after promote" \
        vm_r "test -L /var/lib/edgelet/data/previous"

    assert_ok "split units active after upgrade" \
        lima_wait_split_units "${VM_NAME}" 180

    assert_ok "containerd socket healthy" \
        vm_r "test -S /run/edgelet/containerd.sock"

    assert_ok "edgelet-containerd journal shows ready (no crash loop)" \
        vm_r "journalctl -u edgelet-containerd -n 120 --no-pager | grep -q 'Embedded containerd is ready'"

    assert_ok "containerd config version 4 (1.0.2 embed)" \
        wait_containerd_config_v4

    assert_ok "containerd child PID changed (data plane restarted)" \
        vm_r "test \"\$(systemctl show edgelet-containerd -p MainPID --value)\" != \"${_old_ctd_pid}\""

    assert_ok "${MS_A} reconciled to RUNNING" \
        wait_ms_name_running "${MS_A}" 300

    assert_ok "${MS_B} reconciled to RUNNING" \
        wait_ms_name_running "${MS_B}" 300

    assert_ok "runtime.engineReady after reconcile" \
        lima_wait_edgelet_api "${VM_NAME}" 120
}

run_same_version_upgrade_negative() {
    local _to_host _vm_bin="/var/tmp/edgelet-${TO_VERSION}-noop"
    _to_host="$(release_bin_path "${TO_VERSION}")"

    log_step "Same-version upgrade must not restart data plane (${TO_VERSION})"
    stage_bin_to_vm "${_to_host}" "${_vm_bin}"

    local _log
    _log="$(mktemp)"
    # shellcheck disable=SC2064
    trap "rm -f '${_log}'" RETURN

    vm_r "${BUNDLED_INSTALL} \
        --upgrade \
        --bin-path=${_vm_bin} \
        --version=${TO_VERSION} \
        --arch=${TARGET_ARCH}" 2>&1 | tee "${_log}"

    if grep -q "data-plane restart required" "${_log}"; then
        die "Same-version upgrade must not log data-plane restart required"
    fi
    log_ok "Same-version upgrade skipped data-plane restart"
}

main() {
    echo ""
    echo "======================================================================"
    echo "  OTA embed upgrade (Lima)"
    echo "  VM: ${VM_NAME}  arch: ${TARGET_ARCH}"
    echo "  ${FROM_VERSION} -> ${TO_VERSION}  repo: ${GITHUB_REPO}"
    echo "======================================================================"

    ensure_release_bins
    ensure_vm

    log_step "Preflight: release embed hashes must differ"
    stage_bin_to_vm "$(release_bin_path "${FROM_VERSION}")" "/var/tmp/edgelet-preflight-from"
    stage_bin_to_vm "$(release_bin_path "${TO_VERSION}")" "/var/tmp/edgelet-preflight-to"
    H_FROM="$(embed_hash_for_bin "/var/tmp/edgelet-preflight-from")"
    H_TO="$(embed_hash_for_bin "/var/tmp/edgelet-preflight-to")"
    [[ -n "${H_FROM}" && -n "${H_TO}" ]] || die "Could not read embed hash from release binaries"
    [[ "${H_FROM}" != "${H_TO}" ]] || die "Embed hashes identical (${H_FROM}) — pick releases with different embed bundles"
    log_ok "Embed hash changes across releases: ${H_FROM} -> ${H_TO}"

    fresh_install_baseline

    H_BASELINE="$(embed_hash_for_bin "/usr/local/bin/edgelet")"
    [[ "${H_BASELINE}" == "${H_FROM}" ]] || log_warn "Baseline running embed ${H_BASELINE} != release ${H_FROM} (extract may differ from thin read)"

    OLD_CTD_PID="$(vm_r "systemctl show edgelet-containerd -p MainPID --value")"
    [[ -n "${OLD_CTD_PID}" && "${OLD_CTD_PID}" != "0" ]] || die "Missing edgelet-containerd MainPID before upgrade"

    deploy_ota_workloads

    run_embed_upgrade

    H_AFTER="$(embed_hash_for_bin "/usr/local/bin/edgelet")"
    [[ "${H_AFTER}" == "${H_TO}" ]] || die "Post-upgrade embed hash ${H_AFTER} != expected ${H_TO}"

    assert_post_upgrade "${H_FROM}" "${H_TO}" "${OLD_CTD_PID}"
    run_same_version_upgrade_negative

    print_summary
}

main "$@"
