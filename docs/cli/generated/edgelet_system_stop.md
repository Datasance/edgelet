## edgelet system stop

Gracefully stop the daemon

### Synopsis

Gracefully stop the ioFog Agent daemon (edgelet).

WARNING: Stopping the daemon disables LocalAPI until the daemon is started again
(edgelet or systemctl start edgelet).

```
edgelet system stop [flags]
```

### Options

```
  -h, --help   help for stop
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

* [edgelet system](edgelet_system.md)	 - System operations


