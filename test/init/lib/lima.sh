# test/init/lib/lima.sh — Lima VM helpers (Alpine openrc smoke).
#
# Lima 2.x emits JSONL (one object per line). Do not use '.[] | select(...)' on
# limactl list --json output — that breaks with "Cannot index string with name".
#
# shellcheck shell=bash

lima_vm_status() {
    local name="$1"
    limactl list --json 2>/dev/null \
        | jq -r --arg n "${name}" 'select(.name == $n) | .status' 2>/dev/null \
        | head -1 || true
}

lima_vm_exists() {
    local name="$1"
    [[ -n "$(lima_vm_status "${name}")" ]]
}

lima_vm_running() {
    local name="$1"
    [[ "$(lima_vm_status "${name}")" == "Running" ]]
}
