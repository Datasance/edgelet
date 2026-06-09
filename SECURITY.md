# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| `v1.0.0-beta.1` and later pre-releases on `develop` | Yes |
| Earlier migration / dev builds | No |

## Reporting a vulnerability

If you believe you have found a security issue in Edgelet:

1. **Do not** open a public GitHub issue for exploitable vulnerabilities.
2. Email **security@datasance.com** with:
   - A description of the issue and impact
   - Steps to reproduce (proof-of-concept if available)
   - Affected version / commit and platform (linux embed, docker/podman, desktop)
3. We aim to acknowledge reports within **5 business days** and provide a remediation timeline when confirmed.

For non-security bugs, use the public issue tracker or `CONTRIBUTING.md`.

## Security gates (maintainers)

Before release tags, run:

```bash
make security-code   # gosec on ./cmd ./internal ./pkg
make vulncheck       # govulncheck@v1.1.4 + go mod verify
```

- **gosec** is intentionally **not** in golangci-lint; static analysis is scoped to edgelet module trees.
- **govulncheck** scans `./cmd/... ./internal/... ./pkg/...`. Goal: **zero** vulnerabilities affecting call paths.
- CI: [`.github/workflows/govulncheck.yml`](.github/workflows/govulncheck.yml) (on `go.sum` push, daily cron, manual dispatch).

## Dependency updates (beta)

- Go toolchain: track [Go security releases](https://go.dev/doc/devel/release#security); bump `go` in `go.mod` and CI pins promptly.
- Modules: `go get -u` / Dependabot PRs reviewed against `make vulncheck`.
- Embedded runtime pins (containerd, crun, CNI).

## Known vulnerability exceptions

The following findings are **documented exceptions** accepted for `v1.0.0-beta.1`. They are enforced by `scripts/vulncheck.sh` (keep `ALLOWED_VULNS` in sync with this table).

| ID | CVE | Component | Rationale | Fix timeline |
|----|-----|-----------|-----------|--------------|
| **GO-2026-4887** | CVE-2026-34040 | `github.com/docker/docker` client SDK v27.3.1 | Affects **Docker Engine AuthZ plugins** when request bodies exceed ~1 MiB. Edgelet uses the SDK as a **client** to local docker/podman; typical edge deployments do not enable AuthZ plugins. Upgrading the SDK to v28+ breaks the current API surface; daemon patch (Engine ≥ 29.3.1) is an operator responsibility. | Revisit when `github.com/docker/docker` v29.3.1+ is published as a Go module and API migration is scheduled (post-beta). |
| **GO-2026-4883** | (Moby advisory) | `github.com/docker/docker` client SDK v27.3.1 | Daemon-side plugin privilege validation off-by-one; same client-only usage and no AuthZ plugin dependency as GO-2026-4887. | Same as GO-2026-4887. |

**Operator mitigation (docker/podman engine):** run a patched Docker Engine (≥ 29.3.1) or Podman equivalent; restrict API access; do not rely on AuthZ plugins that inspect full request bodies.

## Exception policy

New exceptions require:

1. Entry in this table (GO ID, CVE if any, component, rationale, fix timeline).
2. Matching ID in `scripts/vulncheck.sh` `ALLOWED_VULNS`.
3. Brief note under **Known limitations** in `CHANGELOG.md` at next release (15-4).

Undocumented findings **fail** `make vulncheck` and CI.
