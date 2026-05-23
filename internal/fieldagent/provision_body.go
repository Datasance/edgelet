package fieldagent

import (
	"fmt"
	"strings"

	"github.com/eclipse-iofog/agent/internal/buildmeta"
	"github.com/eclipse-iofog/agent/internal/config"
)

func buildProvisionRequestBody(provisioningKey string) (map[string]interface{}, error) {
	cfg := config.GetInstance()
	engine := strings.ToLower(strings.TrimSpace(cfg.ContainerEngine))
	flavor := strings.ToLower(strings.TrimSpace(buildmeta.Flavor))

	if err := validateProvisionEngineForFlavor(engine); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"key":    provisioningKey,
		"type":   getArchitectureCode(cfg.Arch),
		"engine": engine,
		"flavor": flavor,
	}, nil
}

func validateProvisionEngineForFlavor(engine string) error {
	for _, allowed := range buildmeta.AllowedEngines() {
		if engine == allowed {
			return nil
		}
	}
	return fmt.Errorf(
		"provisioning blocked: containerEngine %q is not valid for flavor %q (allowed: %s)",
		engine,
		strings.ToLower(strings.TrimSpace(buildmeta.Flavor)),
		buildmeta.AllowedEnginesCSV(),
	)
}
