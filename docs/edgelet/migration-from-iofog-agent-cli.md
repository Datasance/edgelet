# Migration from legacy iofog-agent CLI

This guide helps operators update scripts and runbooks after the **clean-break** Edgelet CLI redesign. There are **no compatibility aliases** — legacy invocations fail with a clear error.

## Product mapping

| Legacy | Edgelet |
|--------|---------|
| `iofog-agent` | `edgelet` |
| `iofog-agentd` | `edgelet daemon` |
| `/etc/iofog-agent/` | `/etc/edgelet/` |
| `iofog-agent deploy … validate` | `edgelet deploy -f … --dry-run` |
| `systemctl start iofog-agentd` | `systemctl start edgelet` |

## Command mapping

| Legacy command | New command | Notes |
|----------------|-------------|-------|
| `iofog-agent status` | `edgelet system status` | Runtime health snapshot |
| `iofog-agent info` | `edgelet system info` | Configuration read path |
| `iofog-agent version` | `edgelet --version` or `edgelet system version` | Combined CLI + daemon output |
| `iofog-agent stop` | `edgelet system stop` | Graceful daemon shutdown |
| `iofog-agent prune` | `edgelet system prune` | System-wide prune modes |
| `iofog-agent start` | **Removed** | Use `edgelet daemon` or `systemctl start edgelet` |
| `iofog-agent cert <base64>` | `edgelet config cert <base64>` | Controller certificate install |
| `iofog-agent switch <profile>` | `edgelet config switch <profile>` | Profiles: `dev`, `prod`, `def` |
| `iofog-agent ms ps` | `edgelet ms ls` | List microservices |
| `iofog-agent deploy apply -f FILE` | `edgelet deploy -f FILE` | Flat deploy only |
| `iofog-agent deploy validate -f FILE` | `edgelet deploy -f FILE --dry-run` | Validate without apply |
| `iofog-agent deploy registry -f FILE` | `edgelet deploy -f FILE` | Auto kind-detect from manifest |
| `iofog-agent deploy runtimeclass -f FILE` | `edgelet deploy -f FILE` | Auto kind-detect from manifest |
| `iofog-agent config set KEY VALUE` | `edgelet config --key value` | Long `--kebab-case` or short alias flags |
| `iofog-agent config get KEY` | `edgelet system info` | No per-key get |

## Global flags

```bash
edgelet system status -o json
edgelet system status --no-color
NO_COLOR=1 edgelet deploy -f ms.yaml
CI=true edgelet deploy -f ms.yaml
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Invalid argument |
| 3 | Unauthorized |
| 4 | Not found |
| 5 | Conflict |
| 6 | Not implemented |
| 10 | Daemon unavailable |
| 1 | Internal error |

When the daemon is not running, commands that require EdgeletAPI exit **10**.

## Shell completion

```bash
edgelet completion bash | sudo tee /etc/bash_completion.d/edgelet
edgelet completion zsh >> ~/.zshrc
edgelet completion fish > ~/.config/fish/completions/edgelet.fish
```

## Example script updates

```bash
# Before
iofog-agent status
iofog-agent deploy apply -f manifest.yaml
iofog-agent ms ps

# After
edgelet system status
edgelet deploy -f manifest.yaml
edgelet ms ls
```

## Verification checklist

```bash
edgelet system status -o json | jq .
edgelet deploy -f test/deployment-yamls/microservice.yaml --dry-run
iofog-agent status; echo exit=$?          # should fail (unknown command)
edgelet status; echo exit=$?              # should fail (unknown command)
```

See [../../CHANGELOG.md](../../CHANGELOG.md) for the full breaking-changes list. CLI reference: [../cli/README.md](../cli/README.md).
