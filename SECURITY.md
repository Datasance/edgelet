# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| `v1.0.0-beta.2` and later pre-releases on `develop` | Yes |
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

`make vulncheck` must pass with **zero** vulnerabilities affecting edgelet call paths. There are **no** active exceptions.

Previously accepted exceptions removed after dependency fixes:

- **GO-2026-5932** — transitive `golang.org/x/crypto/openpgp` via `ocicrypt` / `imgcrypt` on embedded containerd (`containerEngine: edgelet`). Resolved by pinning `github.com/containers/ocicrypt v1.3.2` and `github.com/containerd/imgcrypt/v2 v2.0.3` (ProtonMail `go-crypto/openpgp`).
- **GO-2026-4887** / **GO-2026-4883** — legacy `github.com/docker/docker` client SDK; removed after migrating to `github.com/moby/moby/client@v0.4.1`.

## Exception policy

New exceptions require:

1. Entry in this table (GO ID, CVE if any, component, rationale, fix timeline).
2. Matching ID in `scripts/vulncheck.sh` `ALLOWED_VULNS`.
3. Brief note under **Known limitations** in `CHANGELOG.md` at next release (15-4).

Undocumented findings **fail** `make vulncheck` and CI.
