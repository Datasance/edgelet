## edgelet ms

Microservice operations

### Synopsis

Microservice lifecycle and observability on this agent.

Subcommands: ls, inspect, logs, exec, start, stop, restart, kill, rm.

### Examples

```
edgelet ms ls -o json
  edgelet ms ls --source local
  edgelet ms inspect <uuid>
  edgelet ms logs <uuid> --follow
  edgelet ms exec <uuid> -- /bin/sh
```

### Options

```
  -h, --help   help for ms
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
* [edgelet ms exec](edgelet_ms_exec.md)	 - Execute a command in a microservice
* [edgelet ms inspect](edgelet_ms_inspect.md)	 - Inspect a microservice
* [edgelet ms kill](edgelet_ms_kill.md)	 - Kill a microservice
* [edgelet ms logs](edgelet_ms_logs.md)	 - Stream microservice logs
* [edgelet ms ls](edgelet_ms_ls.md)	 - List microservices
* [edgelet ms restart](edgelet_ms_restart.md)	 - Restart a microservice
* [edgelet ms rm](edgelet_ms_rm.md)	 - Remove a microservice
* [edgelet ms start](edgelet_ms_start.md)	 - Start a microservice
* [edgelet ms stop](edgelet_ms_stop.md)	 - Stop a microservice


