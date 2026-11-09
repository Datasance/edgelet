package airgap

import (
	"bufio"
	_ "embed"
	"strings"
)

//go:embed pins.txt
var pinFile string

// KubernetesVersion is the Kubernetes patch this binary reports.
func KubernetesVersion() string {
	scanner := bufio.NewScanner(strings.NewReader(pinFile))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "kubernetes" {
			return fields[1]
		}
	}
	return ""
}
