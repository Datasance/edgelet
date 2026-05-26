## edgelet auth

Authentication operations

### Synopsis

Inspect and manage LocalAPI authentication tokens.

Subcommands: whoami, tokens, revoke.

### Examples

```
edgelet auth whoami -o json
  edgelet auth tokens -o json
  edgelet auth revoke <jti>
```

### Options

```
  -h, --help   help for auth
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
* [edgelet auth revoke](edgelet_auth_revoke.md)	 - Revoke an auth token
* [edgelet auth tokens](edgelet_auth_tokens.md)	 - List auth tokens
* [edgelet auth whoami](edgelet_auth_whoami.md)	 - Show current auth identity


