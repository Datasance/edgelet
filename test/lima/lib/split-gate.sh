#!/usr/bin/env bash
# test/lima/lib/split-gate.sh — runtime split unit gates. Source only.

# lima_embedded_split_active VM — both split units installed and active.
lima_embedded_split_active() {
    local _vm="$1"
    lima_remote "${_vm}" "systemctl is-active --quiet edgelet-containerd" \
        && lima_remote "${_vm}" "systemctl is-active --quiet edgelet" \
        && lima_remote "${_vm}" "test -f /etc/systemd/system/edgelet-containerd.service"
}

# lima_wait_edgelet_api VM [TIMEOUT] — runtime.engineReady via EdgeletAPI.
lima_wait_edgelet_api() {
    local _vm="$1" _timeout="${2:-180}" _elapsed=0
    while (( _elapsed < _timeout )); do
        if lima_remote "${_vm}" \
            "edgelet system status 2>/dev/null | grep -q 'runtime.engineReady: true'"; then
            return 0
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done
    return 1
}

# lima_wait_split_units VM [TIMEOUT] — poll until edgelet-containerd + edgelet active.
lima_wait_split_units() {
    local _vm="$1" _timeout="${2:-180}" _elapsed=0
    while (( _elapsed < _timeout )); do
        if lima_embedded_split_active "${_vm}"; then
            return 0
        fi
        sleep 2
        _elapsed=$(( _elapsed + 2 ))
    done
    lima_remote "${_vm}" "systemctl status edgelet-containerd edgelet --no-pager" || true
    return 1
}
