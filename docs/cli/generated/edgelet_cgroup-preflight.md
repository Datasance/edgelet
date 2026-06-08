## edgelet cgroup-preflight

Validate cgroup mounts and delegation before start

### Synopsis

Runs cgroup detect + preflight checks used by init start_pre hooks. Does not mutate cgroups.

```
edgelet cgroup-preflight [flags]
```

### Options

```
  -h, --help   help for cgroup-preflight
```

### Options inherited from parent commands

```
      --debug            Debug logging
      --no-color         Disable color and interactive UX
  -o, --output string    Output format: human, json, yaml (default "human")
      --quiet            Suppress interactive progress output
      --socket string    Edgelet API unix socket path
      --timeout string   Request timeout
      --verbose          Verbose logging
```

### SEE ALSO

* [edgelet](edgelet.md)	 - Local CLI for the Edgelet daemon


