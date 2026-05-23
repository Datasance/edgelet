# iofog-agent CLI reference

Operator-facing CLI for the Cobra-based `iofog-agent` command tree.

| Resource | Path |
|----------|------|
| Migration from legacy CLI | [migration-from-legacy-cli.md](migration-from-legacy-cli.md) |
| JSON/YAML output shapes | [output-schemas.md](output-schemas.md) |
| Generated per-command pages | [generated/](generated/) (`make cli-docs`) |

## Quick start

```bash
# Runtime health (requires running iofog-agentd)
iofog-agent system status

# Structured output for automation
iofog-agent system status -o json | jq .
iofog-agent ms ls -o json | jq '.items[] | {uuid, name, state}'

# Apply a manifest (auto kind-detect)
iofog-agent deploy -f microservice.yaml

# Validate only
iofog-agent deploy -f microservice.yaml --dry-run
```

## Global flags

| Flag | Description |
|------|-------------|
| `-o`, `--output` | `human` (default), `json`, or `yaml` |
| `--quiet` | Suppress progress/spinner output on stderr |
| `--verbose` | Verbose logging |
| `--debug` | Debug logging |
| `--socket` | LocalAPI unix socket override |
| `--timeout` | Request timeout |
| `--no-color` | Disable color and interactive UX |
| `--version` | Print combined CLI + daemon version |

Environment variables respected by progress UX:

| Variable | Effect |
|----------|--------|
| `NO_COLOR=1` | Non-interactive output, no ANSI escapes |
| `CI=true` | Non-interactive progress even on a TTY |
| `TERM=dumb` | Non-interactive output |

Human-mode `deploy -f` and `config` show a spinner on stderr while the operation runs, then print a green `✔` success line on stderr (red `✘` when config keys are partially rejected). Structured `-o json` / `-o yaml` output remains on stdout only.

Partial config rejection (some keys accepted, some rejected) exits **2** with full accepted/rejected detail on stderr.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Internal error |
| 2 | Invalid argument |
| 3 | Unauthorized / forbidden |
| 4 | Not found |
| 5 | Conflict |
| 6 | Not implemented |
| 10 | Daemon unavailable (`DAEMON_UNAVAILABLE`) |

When the daemon is not running, commands that require LocalAPI exit **10** with guidance to start `iofog-agentd` or `systemctl start iofog-agentd`.

Remote command exit codes from `ms exec` propagate directly (e.g. container exit 42 → CLI exit 42).

## Command groups

| Group | Commands |
|-------|----------|
| `system` | `status`, `info`, `version`, `reload`, `stop`, `logs`, `prune` |
| `provision` / `deprovision` | top-level |
| `config` | `--long-flag` / `--alias` flags (`--disk-limit-gib`, `--memory-limit-mib`, …), `config cert`, `config switch` |
| `ms` | `ls`, `inspect`, `logs`, `exec`, `start`, `stop`, `restart`, `kill`, `rm` |
| `deploy` | `-f FILE [--dry-run] [--sourceName]` |
| `registry` / `runtimeclass` / `image` | `ls`, `inspect`, `rm` (+ image `pull`, `load`, `prune`) |
| `auth` | `whoami`, `tokens`, `revoke` |

## Examples

```bash
# Config read vs write
iofog-agent system info                    # read configuration
iofog-agent config --network-interface eth0
iofog-agent config --n eth0 --cf 10        # short alias flags
iofog-agent config cert <base64-encoded-cert-string>
iofog-agent config switch prod
iofog-agent -o json config --cf 10       # structured output on stdout

# Microservice lifecycle
iofog-agent ms ls -o json
iofog-agent ms inspect <uuid>
iofog-agent ms logs <uuid> --follow
iofog-agent ms exec <uuid> -- /bin/sh

# Registry / image
iofog-agent registry ls -o json
iofog-agent image pull docker.io/library/alpine:3.19

# Auth
iofog-agent auth whoami -o json
```

## Shell completion

Hidden command: `iofog-agent completion bash|zsh|fish`

```bash
iofog-agent completion bash | sudo tee /etc/bash_completion.d/iofog-agent
```

Regenerate packaging script: `make cli-completion`

## Documentation maintenance

```bash
make cli-docs              # regenerate docs/cli/generated/
make cli-docs-check        # fail if generated docs drift from git
```

Hidden generator: `iofog-agent documentation generate md|man --output DIR`
