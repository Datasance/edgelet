## edgelet system

System operations

### Synopsis

Agent runtime and daemon operations.

Subcommands: status, info, version, reload, stop, logs, prune.

### Options

```
  -h, --help   help for system
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
* [edgelet system info](edgelet_system_info.md)	 - Show agent configuration info
* [edgelet system logs](edgelet_system_logs.md)	 - Stream daemon logs
* [edgelet system prune](edgelet_system_prune.md)	 - Prune unused resources
* [edgelet system reload](edgelet_system_reload.md)	 - Reload daemon configuration
* [edgelet system status](edgelet_system_status.md)	 - Show agent runtime status
* [edgelet system stop](edgelet_system_stop.md)	 - Gracefully stop the daemon
* [edgelet system version](edgelet_system_version.md)	 - Show combined CLI and daemon version


