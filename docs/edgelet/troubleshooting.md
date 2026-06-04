# Edgelet troubleshooting

Common issues when running Edgelet on edge nodes.

## Daemon won't start

**Symptoms:** `systemctl start edgelet` fails; no listening socket on `:54321`.

**Checks:**

1. Validate config paths and permissions:

   ```bash
   ls -la /etc/edgelet/
   edgelet system info
   ```

2. Review journal logs:

   ```bash
   sudo journalctl -u edgelet -n 100 --no-pager
   tail -f /var/log/edgelet/daemon-startup.log
   ```

3. Confirm `containerEngine` is valid on this platform:

   ```bash
   edgelet system version -o json | jq '{allowedEngines, containerEngine}'
   grep containerEngine /etc/edgelet/config.yaml
   ```

   Linux allows `edgelet`, `docker`, or `podman`. Darwin/windows allow `docker` or `podman` only.

4. Check disk space:

   ```bash
   df -h /var/lib/edgelet /var/lib/edgelet-containerd
   ```

---

## Cgroup delegation (embedded edgelet engine)

**Symptoms:** daemon fails at startup with `controller cpu is not available`; `RunPodSandbox` errors; nested Docker deploy fails immediately.

**Checks:**

1. Confirm cgroup mode and driver:

   ```bash
   edgelet system status -o json | jq '{cgroupMode,cgroupDriver,cgroupNested,cgroupDelegatedControllers}'
   ```

2. On **cgroup v2** hosts, verify delegated controllers in the edgelet cgroup:

   ```bash
   grep -E '^(cpu|memory|pids)' /sys/fs/cgroup/cgroup.controllers
   cat /sys/fs/cgroup/cgroup.subtree_control
   ```

3. **Hybrid v1+v2:** edgelet logs a warning and prefers unified v2 — migrate the host to pure cgroup v2 when possible. See [cgroups.md](cgroups.md).

4. **Nested `edgelet-linux` in Docker:** `--privileged` is required:

   ```bash
   docker run -d --name edgelet --privileged \
     -v /var/lib/edgelet:/var/lib/edgelet \
     -v /etc/edgelet:/etc/edgelet \
     ghcr.io/datasance/edgelet-linux:<tag>
   ```

   Without `--privileged`, cpu/memory/pids controllers are not delegated and CRI cannot create sandboxes.

5. **Non-systemd init** (openrc, sysvinit, s6): edgelet uses the **cgroupfs** driver automatically — ensure `/sys/fs/cgroup` is writable and controllers are mounted.

6. **`RunPodSandbox` / `sd-bus call: Invalid unit name or type`:** crun with `SystemdCgroup=true` (manual `config.d` override or an old edgelet build). Reinstall the current edgelet build — generated config must have **`SystemdCgroup = false`** for crun; systemd bare-metal hosts must stay in `edgelet.service` with no `cgroup.path`. Verify:

   ```bash
   edgelet system status -o json | jq '{cgroupDriver,cgroupContainerdPath}'
   grep -E '^\[cgroup\]|path =|SystemdCgroup' /var/lib/edgelet-containerd/config.toml
   cat /proc/$(systemctl show edgelet -p MainPID --value)/cgroup
   ```

7. **`bpf pin to /sys/fs/bpf/crun/k8s_io/...`:** often appears when `SystemdCgroup=true` was enabled for crun. Use a current edgelet build (`SystemdCgroup = false`). If BPF is still required on your host, ensure `/sys/fs/bpf` is mounted and `/sys/fs/bpf/crun/k8s_io` exists (see [cgroups.md](cgroups.md)).

See [cgroups.md](cgroups.md) for the full matrix.

---

## Control vs data plane restart

**Symptoms:** MS disappear after `systemctl restart edgelet`; unexpected downtime during agent OTA.

**Checks:**

1. Identify which unit you restarted:

   ```bash
   systemctl status edgelet edgelet-containerd --no-pager
   ```

2. **docker/podman:** control restart should **not** drain MS (`shutdownPolicy=leave-running` default):

   ```bash
   grep shutdownPolicy /etc/edgelet/config.yaml
   edgelet system status -o json | jq '."runtime.shutdownPolicy", ."runtime.agentPhase"'
   docker ps --filter label=iofog.org/microservice-uuid
   ```

3. **embedded split:** restart **`edgelet` only** for control OTA; restart **`edgelet-containerd`** only when the fat/runtime bundle changed (expect MS stop + reconcile).

4. **Monolithic embedded** (no `edgelet-containerd` active): `restart edgelet` still drains MS — enable runtime split per [workload-continuity.md](workload-continuity.md).

5. **Cold engine change** (Plan 9A): changing `containerEngine` always recreates MS — this is expected and unrelated to workload continuity.

6. **Legacy unit on VM:** if `systemctl cat edgelet-containerd` shows `PartOf=edgelet.service`, reinstall packaging and run `systemctl daemon-reload` — that forces data-plane stop on control restart (T11-C failure).

See [workload-continuity.md](workload-continuity.md) for the full OTA matrix.

---

## Containerd socket (edgelet engine)

**Symptoms:** `connection refused` to `/run/edgelet/containerd.sock`; microservices stuck in pull/create.

**Checks:**

1. Verify socket exists:

   ```bash
   ls -la /run/edgelet/containerd.sock
   ```

2. Check embedded containerd logs in daemon journal output.

3. Look for orphan overlay mounts blocking cleanup:

   ```bash
   mount | grep edgelet
   ```

4. Restart daemon (graceful):

   ```bash
   sudo systemctl restart edgelet
   ```

---

## EdgeletAPI 401 / auth failures

**Symptoms:** CLI exits **3** (Unauthorized); `curl` returns 401.

**Checks:**

1. Token file present and readable:

   ```bash
   sudo ls -la /etc/edgelet/edgelet-api
   ```

2. CLI uses correct CA:

   ```bash
   edgelet auth whoami
   edgelet auth whoami -o json
   ```

3. After provisioning, ensure you are not sending an unsigned bootstrap JWT.

4. Regenerate PKI only when directed — mismatched CA breaks existing CLI sessions.

---

## CLI connectivity (exit 10)

**Symptoms:** `Error[DAEMON_UNAVAILABLE]`; exit code **10**.

**Checks:**

1. Daemon running:

   ```bash
   systemctl is-active edgelet
   edgelet system status
   ```

2. Socket override — if using `--socket`, path must match `/run/edgelet/edgelet.sock`.

3. Firewall — EdgeletAPI listens on localhost/TLS; remote access is not the default operator path.

---

## External Docker / Podman

**Symptoms:** `Cannot connect to Docker daemon`; containers not starting.

```bash
sudo systemctl status docker    # or podman.socket
ls -la /var/run/docker.sock
grep dockerUrl /etc/edgelet/config.yaml
```

Ensure the configured `dockerUrl` matches the running engine socket.

---

## Controller connectivity

**Symptoms:** `connectionToController` not ok in `edgelet system status`.

```bash
edgelet system status -o json | jq .connectionToController
curl -v "$(grep ^controller: /etc/edgelet/config.yaml | awk '{print $2}')/health"
edgelet config cert <base64-controller-cert>   # if TLS verification fails
```

---

## Embedded integration tests

For embedded-engine regressions on macOS, use the Lima VM pipeline:

```bash
./test/embedded/run-all.sh
```

See [../../test/embedded/README.md](../../test/embedded/README.md).

---

## Collecting diagnostics

```bash
sudo journalctl -u edgelet --since "1 hour ago" > edgelet-journal.log
edgelet system status -o json > edgelet-status.json
edgelet system info -o json > edgelet-info.json
edgelet system version -o json > edgelet-version.json
```

Do not share `/etc/edgelet/edgelet-api` or private keys in support tickets.
