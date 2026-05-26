## edgelet provision

Provision the agent

### Synopsis

Register this agent with an ioFog Controller using a provisioning key.

The key is issued by the Controller when creating or enrolling an agent.

```
edgelet provision <provisioning-key> [flags]
```

### Examples

```
edgelet provision <provisioning-key>
  edgelet -o json provision <provisioning-key>
```

### Options

```
  -h, --help   help for provision
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


