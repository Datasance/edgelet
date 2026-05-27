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
