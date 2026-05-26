## edgelet ms logs

Stream microservice logs

```
edgelet ms logs <id> [flags]
```

### Options

```
  -f, --follow         Follow log output
  -h, --help           help for logs
      --since string   Show logs since ISO8601 timestamp
      --tail string    Number of lines to show from the end (default "100")
      --timestamps     Show log timestamps
      --until string   Show logs until ISO8601 timestamp
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


