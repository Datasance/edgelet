## edgelet image rm

Remove an image

### Synopsis

Remove an image by repository:tag, image ID, or image ID prefix.

The selector may be a full or short image ID (from edgelet image ls), a unique ID prefix
(at least 3 hexadecimal characters), or a repository tag such as alpine:3.19.

```
edgelet image rm <selector> [flags]
```

### Options

```
  -h, --help   help for rm
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


