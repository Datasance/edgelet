## edgelet init-config

Write default config if missing

### Synopsis

Write the Edgelet default configuration template when no config file exists.

Does not overwrite an existing config. Uses /usr/share/edgelet/edgelet-config.yaml.sample
when installed, otherwise the embedded release template.

```
edgelet init-config [flags]
```

### Examples

```
  sudo edgelet init-config
  edgelet init-config --config-path /etc/edgelet/config.yaml
```

### Options

```
      --config-path string   Config file path (default: /etc/edgelet/config.yaml)
  -h, --help                 help for init-config
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


