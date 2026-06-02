# T10-C — RHEL/sysvinit manual gate (Plan 10)

> **Spec:** [.cursor/edgelet/docs/10-init-systems-enterprise.md](../../.cursor/edgelet/docs/10-init-systems-enterprise.md)

Manual checklist for Tier 2 sysvinit on RHEL-family distros. Mark each item when validated on a lab node.

## Install

- [ ] `install.sh` detects sysvinit and installs `/etc/init.d/edgelet`
- [ ] Config at `/etc/edgelet/config.yaml`; binary at `/usr/local/bin/edgelet`
- [ ] `edgelet cgroup-preflight` succeeds (or documented skip on v1-only VM)

## Service lifecycle

- [ ] `/etc/init.d/edgelet start` — daemon running; status API responds
- [ ] Deploy one MS; container reaches running
- [ ] `/etc/init.d/edgelet stop` — invokes `edgelet-shutdown`; clean exit within kill timeout
- [ ] `/etc/init.d/edgelet status` — correct exit codes

## Cgroup / engine (embedded)

- [ ] Status shows `cgroupDriver=cgroupfs` (non-systemd)
- [ ] Documented: no DelegateSubgroup on sysv

## Sign-off

| Field | Value |
|-------|-------|
| Distro / version | |
| Tester | |
| Date | |
| Pass / fail | |
