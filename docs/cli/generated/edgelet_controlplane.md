## edgelet controlplane

Control plane controller operations

### Synopsis

Inspect, restart, or remove the singleton ControlPlane controller deployment on this node.

Deploy or upgrade with edgelet deploy -f <controlplane.yaml> (kind: ControlPlane). Use restart to bounce the controller container without a full redeploy. There is no controlplane apply subcommand.

### Examples

```
edgelet controlplane get
  edgelet controlplane get --manifest
  edgelet controlplane get -o json
  edgelet controlplane restart
  edgelet controlplane restart --pull
  edgelet controlplane delete
```

### Options

```
  -h, --help   help for controlplane
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
* [edgelet controlplane delete](edgelet_controlplane_delete.md)	 - Remove the control plane deployment
* [edgelet controlplane get](edgelet_controlplane_get.md)	 - Show control plane deployment status or masked manifest
* [edgelet controlplane restart](edgelet_controlplane_restart.md)	 - Restart the control plane controller container


