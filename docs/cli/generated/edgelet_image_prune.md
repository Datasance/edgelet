## edgelet image prune

Prune dangling images

### Synopsis

Prune dangling images only.

```
edgelet image prune [dangling] [flags]
```

### Examples

```
edgelet image prune
edgelet image prune dangling
edgelet image prune --mode dangling
```

### Options

```
  -h, --help          help for prune
  -m, --mode string   Prune mode (only: dangling)
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

* [edgelet image](edgelet_image.md)	 - Image operations


