package provision

import "strings"

// ProvisionLong returns the provision command introduction.
func ProvisionLong() string {
	return strings.TrimSpace(`Register this agent with an ioFog Controller using a provisioning key.

The key is issued by the Controller when creating or enrolling an agent.`)
}

// ProvisionExamples returns provision command examples for Cobra.
func ProvisionExamples() string {
	return strings.TrimSpace(`iofog-agent provision <provisioning-key>
  iofog-agent -o json provision <provisioning-key>`)
}

// DeprovisionLong returns the deprovision command introduction.
func DeprovisionLong() string {
	return strings.TrimSpace(`Remove agent provisioning and begin cleanup of managed resources.

WARNING: Deprovisioning stops controller management and may remove managed microservices.
Use --keep-local or --scope local to preserve locally deployed microservices.`)
}

// DeprovisionExamples returns deprovision command examples for Cobra.
func DeprovisionExamples() string {
	return strings.TrimSpace(`iofog-agent deprovision
  iofog-agent deprovision --scope local
  iofog-agent deprovision --keep-local`)
}
