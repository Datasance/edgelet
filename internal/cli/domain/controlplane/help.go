package controlplane

import "strings"

// CommandLong returns the controlplane command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Inspect, restart, or remove the singleton ControlPlane controller deployment on this node.

Deploy or upgrade with edgelet deploy -f <controlplane.yaml> (kind: ControlPlane). Use restart to bounce the controller container without a full redeploy. There is no controlplane apply subcommand.`)
}

// CommandExamples returns controlplane command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`edgelet controlplane get
  edgelet controlplane get --manifest
  edgelet controlplane get -o json
  edgelet controlplane restart
  edgelet controlplane restart --pull
  edgelet controlplane delete`)
}
