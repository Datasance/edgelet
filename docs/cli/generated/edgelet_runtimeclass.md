## edgelet runtimeclass

Runtime class operations

### Synopsis

Manage runtime class definitions for local microservice deploy manifests.

Subcommands: ls, inspect, rm.

### Examples

```
edgelet runtimeclass ls -o json
  edgelet runtimeclass inspect <name>
  edgelet runtimeclass rm <name>
```

### Options

```
  -h, --help   help for runtimeclass
```

### Options inherited from parent commands

```
      --debug            Debug logging
      --no-color         Disable color and interactive UX
  -o, --output string    Output format: human, json, yaml (default "human")
      --quiet            Suppress interactive progress output
      --socket string    LocalAPI unix socket path
      --timeout string   Request timeout
      --verbose          Verbose logging
```

### SEE ALSO

* [edgelet](edgelet.md)	 - Local CLI for the Edgelet daemon
* [edgelet runtimeclass inspect](edgelet_runtimeclass_inspect.md)	 - Inspect a runtime class
* [edgelet runtimeclass ls](edgelet_runtimeclass_ls.md)	 - List runtime classes
* [edgelet runtimeclass rm](edgelet_runtimeclass_rm.md)	 - Remove a runtime class


