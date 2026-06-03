package controlplane

import "strings"

// CommandLong returns the controlplane command group introduction.
func CommandLong() string {
	return strings.TrimSpace(`Inspect or remove the singleton ControlPlane controller deployment on this node.

Deploy with edgelet deploy -f <controlplane.yaml> (kind: ControlPlane). There is no controlplane apply subcommand.`)
}

// CommandExamples returns controlplane command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`edgelet controlplane get
  edgelet controlplane get --manifest
  edgelet controlplane get -o json
  edgelet controlplane delete`)
}
