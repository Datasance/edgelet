## edgelet runtime drain

Drain labeled microservice containers before data-plane stop

### Synopsis

Stops ioFog microservice containers via the control plane while the CRI socket is still up.

Default timeout follows shutdownGracePeriodSeconds (90s). Exit 0 when drain completes; exit 1 on timeout.

```
edgelet runtime drain [flags]
```

### Options

```
  -h, --help          help for drain
      --timeout int   Drain budget in seconds (0 = server default) (default 90)
```

### Options inherited from parent commands

```
      --debug           Debug logging
      --no-color        Disable color and interactive UX
  -o, --output string   Output format: human, json, yaml (default "human")
      --quiet           Suppress interactive progress output
      --socket string   Edgelet API unix socket path
      --verbose         Verbose logging
```

### SEE ALSO

* [edgelet runtime](edgelet_runtime.md)	 - Embedded runtime data-plane operations


