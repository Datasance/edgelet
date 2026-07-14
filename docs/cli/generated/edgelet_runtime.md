## edgelet runtime

Embedded runtime data-plane operations

### Synopsis

Operations for the embedded containerd data plane.

Used by edgelet-containerd.service stop hooks and operators during maintenance.

### Options

```
  -h, --help   help for runtime
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
* [edgelet runtime drain](edgelet_runtime_drain.md)	 - Drain labeled microservice containers before data-plane stop
* [edgelet runtime reap-orphans](edgelet_runtime_reap-orphans.md)	 - Reap orphaned edgelet data-plane shims and containerd children


