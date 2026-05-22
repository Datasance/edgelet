# Migration from Legacy iofog-agent CLI

This guide helps operators update scripts, runbooks, and muscle memory after the **clean-break** Cobra CLI redesign. There are **no compatibility aliases** — legacy invocations fail with a clear error.

For Java-to-Go agent migration, see [migration.md](../migration.md).

## Quick reference

| Legacy command | New command | Notes |
|----------------|-------------|-------|
| `iofog-agent status` | `iofog-agent system status` | Runtime health snapshot |
| `iofog-agent info` | `iofog-agent system info` | Configuration read path |
| `iofog-agent version` | `iofog-agent --version` or `iofog-agent system version` | Identical combined CLI + daemon output |
| `iofog-agent stop` | `iofog-agent system stop` | Graceful daemon shutdown |
| `iofog-agent prune` | `iofog-agent system prune` | System-wide prune modes |
| `iofog-agent start` | **Removed** | Use `iofog-agentd` or `systemctl start iofog-agentd` |
| `iofog-agent cert <base64>` | `iofog-agent config cert <base64>` | Controller certificate install |
| `iofog-agent switch <profile>` | `iofog-agent config switch <profile>` | Profiles: `dev`, `prod`, `def` (+ aliases) |
| `iofog-agent ms ps` | `iofog-agent ms ls` | List microservices |
| `iofog-agent deploy apply -f FILE` | `iofog-agent deploy -f FILE` | Flat deploy only |
| `iofog-agent deploy validate -f FILE` | `iofog-agent deploy -f FILE --dry-run` | Validate without apply |
| `iofog-agent deploy registry -f FILE` | `iofog-agent deploy -f FILE` | Auto kind-detect from manifest |
| `iofog-agent deploy runtimeclass -f FILE` | `iofog-agent deploy -f FILE` | Auto kind-detect from manifest |
| `iofog-agent config set KEY VALUE` | `iofog-agent config KEY VALUE` | `set` subcommand removed |
| `iofog-agent config get KEY` | `iofog-agent system info` | No per-key get |

## Global flags (new)

```bash
iofog-agent system status -o json          # structured output
iofog-agent system status --no-color       # disable interactive UX
NO_COLOR=1 iofog-agent deploy -f ms.yaml   # no ANSI progress
CI=true iofog-agent deploy -f ms.yaml      # non-interactive progress on TTY
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

When the daemon is not running, commands that require LocalAPI now exit **10** (previously some paths exited 0).

## Shell completion

```bash
# Bash (system-wide, requires root)
iofog-agent completion bash | sudo tee /etc/bash_completion.d/iofog-agent

# Bash (user)
iofog-agent completion bash >> ~/.bashrc

# Zsh
iofog-agent completion zsh >> ~/.zshrc

# Fish
iofog-agent completion fish > ~/.config/fish/completions/iofog-agent.fish
```

Packaged installs via `install.sh` drop a generated script at `/etc/bash_completion.d/iofog-agent` when the directory exists.

## Streaming commands

- `iofog-agent ms logs --follow` — human log stream only; `-o json|yaml` is rejected
- `iofog-agent ms exec` — TTY attach; remote exit code propagates (e.g. exit 42 → CLI exit 42)

## Example script updates

```bash
# Before
iofog-agent status
iofog-agent deploy apply -f manifest.yaml
iofog-agent ms ps

# After
iofog-agent system status
iofog-agent deploy -f manifest.yaml
iofog-agent ms ls
```

## Verification checklist

```bash
iofog-agent system status -o json | jq .
iofog-agent deploy -f test/deployment-yamls/microservice.yaml --dry-run
iofog-agent status; echo exit=$?          # should fail (unknown command)
iofog-agent deploy apply -f file.yaml; echo exit=$?  # should fail
iofog-agent config set foo bar; echo exit=$?         # should fail
```

See [CHANGELOG.md](../../CHANGELOG.md) for the full breaking-changes list.
