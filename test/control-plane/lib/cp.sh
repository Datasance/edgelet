#!/usr/bin/env bash
# test/control-plane/lib/cp.sh — ControlPlane integration tests helpers. Source only.

# shellcheck shell=bash

CP_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CP_REPO_ROOT="$(cd "${CP_LIB_DIR}/../../.." && pwd)"

# shellcheck source=test/lima/lib/log.sh
source "${CP_REPO_ROOT}/test/lima/lib/log.sh"
# shellcheck source=test/lima/lib/arch.sh
source "${CP_REPO_ROOT}/test/lima/lib/arch.sh"
# shellcheck source=test/lima/lib/remote.sh
source "${CP_REPO_ROOT}/test/lima/lib/remote.sh"
# shellcheck source=test/workload-continuity/lib/ensure-vm.sh
source "${CP_REPO_ROOT}/test/workload-continuity/lib/ensure-vm.sh"

CP_DEFAULT_FIXTURE="${CP_REPO_ROOT}/test/control-plane/fixtures/controlplane-it.yaml"
CP_NS=""
CP_NAME=""

# cp_fixture_metadata FIXTURE — sets CP_NS and CP_NAME from YAML metadata block.
cp_fixture_metadata() {
    local _fixture="$1"
    [[ -f "${_fixture}" ]] || die "Missing fixture ${_fixture}"
    CP_NS="$(awk '/^metadata:/{m=1; next} m && /^[^[:space:]]/{m=0} m && /^[[:space:]]+namespace:/{print $2; exit}' "${_fixture}")"
    CP_NAME="$(awk '/^metadata:/{m=1; next} m && /^[^[:space:]]/{m=0} m && /^[[:space:]]+name:/{print $2; exit}' "${_fixture}")"
    [[ -n "${CP_NS}" && -n "${CP_NAME}" ]] \
        || die "fixture must set metadata.namespace and metadata.name (${_fixture})"
}

# cp_fqdn_* — three locked FQDNs from fixture identity.
cp_fqdn_edgelet_controller() { echo "edgelet.controller.svc.bridge.local"; }
cp_fqdn_controller_namespace() { echo "controller.${CP_NS}.svc.bridge.local"; }
cp_fqdn_standard() { echo "${CP_NS}.${CP_NAME}.svc.bridge.local"; }

cp_remote() { lima_remote "$@"; }

cp_scp_fixture() {
    local _vm="$1" _fixture="$2"
    local _ssh_config _ssh_host
    _ssh_config="$(lima_ssh_config "${_vm}")"
    _ssh_host="$(lima_ssh_host "${_vm}")"
    scp -F "${_ssh_config}" -q "${_fixture}" "${_ssh_host}:/tmp/controlplane-it.yaml"
}

# cp_cleanup_stale_runtime_artifacts VM — drop orphaned edgelet_* CRI/docker leftovers (embedded IT).
cp_cleanup_stale_runtime_artifacts() {
    local _vm="$1"
    cp_remote "${_vm}" "
        set +e
        if edgelet system status 2>/dev/null | grep -q 'runtime.engine: edgelet'; then
            SOCK=/run/edgelet/containerd.sock
            NS=k8s.io
            if command -v ctr >/dev/null 2>&1 && [ -S \"\${SOCK}\" ]; then
                for cid in \$(ctr --address \"\${SOCK}\" -n \"\${NS}\" containers list -q 2>/dev/null); do
                    info=\$(ctr --address \"\${SOCK}\" -n \"\${NS}\" containers info \"\${cid}\" 2>/dev/null || true)
                    echo \"\${info}\" | grep -q 'edgelet_' || continue
                    ctr --address \"\${SOCK}\" -n \"\${NS}\" tasks kill -s SIGKILL \"\${cid}\" 2>/dev/null || true
                    ctr --address \"\${SOCK}\" -n \"\${NS}\" containers delete \"\${cid}\" 2>/dev/null || true
                done
                for sid in \$(ctr --address \"\${SOCK}\" -n \"\${NS}\" sandboxes list -q 2>/dev/null); do
                    info=\$(ctr --address \"\${SOCK}\" -n \"\${NS}\" sandboxes info \"\${sid}\" 2>/dev/null || true)
                    echo \"\${info}\" | grep -q 'edgelet_' || continue
                    ctr --address \"\${SOCK}\" -n \"\${NS}\" sandboxes rm \"\${sid}\" 2>/dev/null || true
                done
            fi
        elif edgelet system status 2>/dev/null | grep -q 'runtime.engine: docker'; then
            ids=\$(docker ps -aq --filter name=edgelet_ 2>/dev/null || true)
            if [ -n \"\${ids}\" ]; then
                docker rm -f \${ids} 2>/dev/null || true
            fi
        fi
    " 2>/dev/null || true
}

# cp_delete_if_present VM — idempotent cleanup before deploy.
cp_delete_if_present() {
    local _vm="$1"
    cp_remote "${_vm}" "
        set +e
        if edgelet controlplane get >/dev/null 2>&1; then
            edgelet controlplane delete >/dev/null 2>&1 || true
        fi
        for i in \$(seq 1 45); do
            edgelet controlplane get >/dev/null 2>&1 || break
            sleep 2
        done
        if edgelet controlplane get >/dev/null 2>&1; then
            exit 1
        fi
        for i in \$(seq 1 30); do
            edgelet ms ls --source controlplane 2>/dev/null | grep -qi running || exit 0
            sleep 2
        done
        exit 0
    " 2>/dev/null || true
    cp_cleanup_stale_runtime_artifacts "${_vm}"
}

# cp_deploy VM [FIXTURE] — async apply must exit 0 (poll + runtimeState=running).
cp_deploy() {
    local _vm="$1"
    local _fixture="${2:-${CP_DEFAULT_FIXTURE}}"
    cp_fixture_metadata "${_fixture}"
    cp_delete_if_present "${_vm}"
    cp_scp_fixture "${_vm}" "${_fixture}"
    log_info "Deploying ControlPlane (${CP_NS}/${CP_NAME}) on ${_vm}"
    cp_remote "${_vm}" "
        set -e
        edgelet deploy -f /tmp/controlplane-it.yaml
    " || die "edgelet deploy -f failed on ${_vm}"
}

# cp_wait_running VM — controlplane ms + controller /api/v3/status (outcome gate).
cp_wait_running() {
    local _vm="$1"
    local _elapsed=0 _timeout=600
    while (( _elapsed < _timeout )); do
        if cp_remote "${_vm}" "
            set -e
            edgelet ms ls --source controlplane 2>/dev/null | grep -qi running
            out=\$(edgelet controlplane get 2>/dev/null || true)
            echo \"\${out}\" | grep -qi 'runtimeState: running'
            curl -sf http://127.0.0.1:51121/api/v3/status | grep -q '\"status\"'
        " 2>/dev/null; then
            return 0
        fi
        sleep 5
        _elapsed=$(( _elapsed + 5 ))
    done
    die "control plane not running on ${_vm} after ${_timeout}s"
}

# cp_controlplane_uuid VM
cp_controlplane_uuid() {
    local _vm="$1"
    cp_remote "${_vm}" \
        "edgelet ms ls --source controlplane 2>/dev/null | awk '\$3==\"${CP_NAME}\" && \$2==\"${CP_NS}\"{print \$1; exit}'" \
        | tr -d '\r'
}

# cp_assert_deployed VM — embedded CP apply/B container running checks.
cp_assert_deployed() {
    local _vm="$1"
    assert_ok "controlplane get shows namespace=${CP_NS} name=${CP_NAME}" \
        cp_remote "${_vm}" "
            set -e
            out=\$(edgelet controlplane get 2>/dev/null)
            echo \"\${out}\" | grep -q 'namespace: ${CP_NS}'
            echo \"\${out}\" | grep -q 'name: ${CP_NAME}'
            echo \"\${out}\" | grep -qi 'running'
        "
    assert_ok "ms ls --source controlplane lists application=${CP_NS} name=${CP_NAME}" \
        cp_remote "${_vm}" "
            set -e
            out=\$(edgelet ms ls --source controlplane)
            echo \"\${out}\" | grep -q '${CP_NS}'
            echo \"\${out}\" | grep -q '${CP_NAME}'
            echo \"\${out}\" | grep -qi 'controlplane'
            echo \"\${out}\" | grep -qi 'running'
        "
}

# cp_assert_status_api VM — controller status API
cp_assert_status_api() {
    local _vm="$1"
    assert_ok "GET :51121/api/v3/status returns online" \
        cp_remote "${_vm}" "
            set -e
            body=\$(curl -sf http://127.0.0.1:51121/api/v3/status)
            echo \"\${body}\" | grep -Eiq '\"status\"[[:space:]]*:[[:space:]]*\"online\"'
        "
}

# cp_assert_lifecycle VM — CP lifecycle guards (leaves CP deleted)
# Poll limits align with cp_delete_if_present (docker engine delete can be slow under load).
cp_assert_lifecycle() {
    local _vm="$1"
    local _uuid
    _uuid="$(cp_controlplane_uuid "${_vm}")"
    [[ -n "${_uuid}" ]] || die "no controlplane uuid on ${_vm}"

    assert_ok "ms rm on controlplane uuid is rejected" \
        cp_remote "${_vm}" "
            set +e
            out=\$(edgelet ms rm '${_uuid}' 2>&1)
            code=\$?
            set -e
            test \"\${code}\" -ne 0
            echo \"\${out}\" | grep -Eiq 'controlplane|control plane'
        "

    assert_ok "controlplane delete succeeds" \
        cp_remote "${_vm}" "
            set -e
            out=\$(edgelet controlplane delete 2>&1) || { echo \"\${out}\"; exit 1; }
            echo \"\${out}\" | grep -Eiq 'ok|deleted|removed'
            for i in \$(seq 1 45); do
                edgelet controlplane get >/dev/null 2>&1 || exit 0
                sleep 2
            done
            edgelet controlplane get 2>&1 || true
            exit 1
        "

    assert_ok "ms ls --source controlplane empty after delete" \
        cp_remote "${_vm}" "
            set -e
            for i in \$(seq 1 45); do
                out=\$(edgelet ms ls --source controlplane 2>/dev/null || true)
                if ! echo \"\${out}\" | grep -qi running; then
                    exit 0
                fi
                sleep 2
            done
            edgelet ms ls --source controlplane 2>/dev/null || true
            exit 1
        "
}

# cp_deploy_dns_probe VM — busybox for nslookup from bridge network.
cp_deploy_dns_probe() {
    local _vm="$1"
    cp_remote "${_vm}" "
        set -e
        cat >/tmp/cp-dns-probe.yaml <<'EOF'
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: cp-dns-probe
spec:
  image: docker.io/library/busybox:1.36
  registry: 1
  container:
    hostNetworkMode: false
    isPrivileged: false
    commands:
      - /bin/sh
      - -lc
      - sleep 1800
  schedule: 50
EOF
        set +e
        edgelet deploy -f /tmp/cp-dns-probe.yaml >/dev/null 2>&1
        for i in \$(seq 1 40); do
            edgelet ms ls --source local 2>/dev/null | grep -F 'cp-dns-probe' | grep -qi running && exit 0
            sleep 3
        done
        exit 1
    "
}

cp_dns_probe_uuid() {
    local _vm="$1"
    cp_remote "${_vm}" \
        "edgelet ms ls --source local 2>/dev/null | awk '\$3==\"cp-dns-probe\"{print \$1; exit}'" \
        | tr -d '\r'
}

# cp_assert_dns VM — CP DNS resolution (embedded engine resolver)
cp_assert_dns() {
    local _vm="$1"
    local _probe _fqdn1 _fqdn2 _fqdn3
    _fqdn1="$(cp_fqdn_edgelet_controller)"
    _fqdn2="$(cp_fqdn_controller_namespace)"
    _fqdn3="$(cp_fqdn_standard)"
    cp_deploy_dns_probe "${_vm}"
    _probe="$(cp_dns_probe_uuid "${_vm}")"
    [[ -n "${_probe}" ]] || die "dns probe ms missing on ${_vm}"

    assert_ok "DNS resolves ${_fqdn1}" \
        cp_remote "${_vm}" "
            set -e
            for i in \$(seq 1 25); do
                edgelet ms exec '${_probe}' -- nslookup '${_fqdn1}' >/dev/null 2>&1 && exit 0
                sleep 3
            done
            exit 1
        "
    assert_ok "DNS resolves ${_fqdn2}" \
        cp_remote "${_vm}" "
            set -e
            edgelet ms exec '${_probe}' -- nslookup '${_fqdn2}' >/dev/null 2>&1
        "
    assert_ok "DNS resolves ${_fqdn3}" \
        cp_remote "${_vm}" "
            set -e
            edgelet ms exec '${_probe}' -- nslookup '${_fqdn3}' >/dev/null 2>&1
        "

    cp_remote "${_vm}" "edgelet ms rm '${_probe}' >/dev/null 2>&1 || true" 2>/dev/null || true
}

# cp_ensure_docker_vm REPO VM SKIP_SETUP
cp_ensure_docker_vm() {
    local _root="$1" _vm="$2" _skip_setup="$3"
    command -v limactl >/dev/null || die "limactl required for docker CP apply"
    if [[ "${_skip_setup}" != true ]]; then
        "${_root}/test/engine-lifecycle/setup.sh"
    fi
    "${_root}/test/engine-lifecycle/vm-start.sh" --vm-name="${_vm}"
    if cp_remote "${_vm}" "
        systemctl is-active --quiet edgelet && \
        edgelet system status 2>/dev/null | grep -q 'runtime.engine: docker'
    " 2>/dev/null; then
        log_info "Docker VM ${_vm} already has edgelet+docker engine"
        wc_wait_edgelet_api "${_vm}" 120 || die "edgelet API not ready"
        return 0
    fi
    log_step "Installing edgelet with docker engine on ${_vm}"
    "${_root}/test/engine-lifecycle/vm-install.sh" --vm-name="${_vm}" --start-engine=docker
    wc_wait_edgelet_api "${_vm}" 120 || die "edgelet API not ready after docker install"
}

# cp_ensure_embedded_vm REPO VM ARCH SKIP_SETUP
cp_ensure_embedded_vm() {
    local _root="$1" _vm="$2" _arch="$3" _skip_setup="$4"
    command -v limactl >/dev/null || die "limactl required for embedded CP apply"
    "${_root}/test/embedded/vm-start.sh" --vm-name="${_vm}"
    if cp_remote "${_vm}" "
        systemctl is-active --quiet edgelet-containerd && \
        systemctl is-active --quiet edgelet && \
        edgelet system status 2>/dev/null | grep -q 'runtime.engine: edgelet'
    " 2>/dev/null; then
        log_info "Embedded split already active on ${_vm}"
        wc_wait_edgelet_api "${_vm}" 120 || die "edgelet API not ready"
        return 0
    fi
    if [[ "${_skip_setup}" != true ]]; then
        "${_root}/test/embedded/setup.sh"
    fi
    wc_install_embedded_split "${_root}" "${_vm}" "${_arch}" "dev-cp-it"
    wc_wait_edgelet_api "${_vm}" 120 || die "edgelet API not ready after embedded install"
}
