package deploy

import "strings"

// CommandLong returns the deploy command introduction.
func CommandLong() string {
	return strings.TrimSpace(`Apply or validate a local microservice, registry, or runtimeclass manifest via LocalAPI v3.

Manifest kind is auto-detected from the YAML file.`)
}

// CommandExamples returns deploy command examples for Cobra.
func CommandExamples() string {
	return strings.TrimSpace(`iofog-agent deploy -f microservice.yaml
  iofog-agent deploy -f microservice.yaml --dry-run
  iofog-agent deploy -f microservice.yaml --sourceName my-app
  iofog-agent deploy -f registry.yaml
  iofog-agent -o json deploy -f microservice.yaml --dry-run`)
}
