## edgelet ms kill

Kill a microservice

### Synopsis

Forcefully terminate a microservice container.

WARNING: kill sends SIGKILL-equivalent termination; in-flight work may be lost.

```
edgelet ms kill <id> [flags]
```

### Options

```
  -h, --help   help for kill
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

* [edgelet ms](edgelet_ms.md)	 - Microservice operations


