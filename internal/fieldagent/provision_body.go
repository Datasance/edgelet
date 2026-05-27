package fieldagent

import (
	"fmt"
	"strings"

	"github.com/datasance/edgelet/internal/buildmeta"
	"github.com/datasance/edgelet/internal/config"
)

const provisionFlavor = "edgelet"

func buildProvisionRequestBody(provisioningKey string) (map[string]interface{}, error) {
	cfg := config.GetInstance()
	engine := strings.ToLower(strings.TrimSpace(cfg.ContainerEngine))

	if err := validateProvisionEngine(engine); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"key":    provisioningKey,
		"type":   getArchitectureCode(cfg.Arch),
		"engine": engine,
		"flavor": provisionFlavor,
	}, nil
}

func validateProvisionEngine(engine string) error {
	for _, allowed := range buildmeta.AllowedEngines() {
		if engine == allowed {
			return nil
		}
	}
	return fmt.Errorf(
		"provisioning blocked: containerEngine %q is not valid on this platform (allowed: %s)",
		engine,
		buildmeta.AllowedEnginesCSV(),
	)
}
