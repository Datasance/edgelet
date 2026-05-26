## edgelet completion

Generate shell completion scripts

### Synopsis

Generate shell completion scripts for edgelet.

Install the script for your shell, then start a new session or source the file.

bash:
  edgelet completion bash | sudo tee /etc/bash_completion.d/edgelet

zsh:
  edgelet completion zsh > "${fpath[1]}/_edgelet"

fish:
  edgelet completion fish > ~/.config/fish/completions/edgelet.fish

Regenerate the packaged bash script with: make cli-completion

### Examples

```
edgelet completion bash
  edgelet completion zsh
  edgelet completion fish
```

### Options

```
  -h, --help   help for completion
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
* [edgelet completion bash](edgelet_completion_bash.md)	 - Generate bash completion script
* [edgelet completion fish](edgelet_completion_fish.md)	 - Generate fish completion script
* [edgelet completion zsh](edgelet_completion_zsh.md)	 - Generate zsh completion script


