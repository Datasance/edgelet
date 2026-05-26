## edgelet completion bash

Generate bash completion script

```
edgelet completion bash [flags]
```

### Examples

```
edgelet completion bash | sudo tee /etc/bash_completion.d/edgelet
```

### Options

```
  -h, --help   help for bash
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

* [edgelet completion](edgelet_completion.md)	 - Generate shell completion scripts


