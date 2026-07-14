## edgelet runtime reap-orphans

Reap orphaned edgelet data-plane shims and containerd children

### Synopsis

Last-resort cleanup for edgelet-scoped containerd-shim and --edgelet-containerd-child
processes bound to /run/edgelet/containerd.sock. Used by edgelet-containerd stop hooks and operators during recovery.

```
edgelet runtime reap-orphans [flags]
```

### Options

```
  -h, --help   help for reap-orphans
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

* [edgelet runtime](edgelet_runtime.md)	 - Embedded runtime data-plane operations


