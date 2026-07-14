## edgelet shutdown

Control-plane stop for init systems

### Synopsis

Gracefully stops the edgelet daemon. Used by systemd ExecStop and tier-1/2 init scripts. Control-plane stop skips microservice drain when shutdownPolicy=leave-running (default for docker/podman and embedded split).

```
edgelet shutdown [flags]
```

### Options

```
  -h, --help   help for shutdown
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


