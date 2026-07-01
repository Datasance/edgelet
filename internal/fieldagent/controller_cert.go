package fieldagent

import (
	"crypto/tls"
	"errors"
	"fmt"
	"os"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

// buildControllerTLSConfig loads controllerCert from config and builds TLS settings
// for controller HTTPS/WSS dials.
func buildControllerTLSConfig(secureMode bool, configuredPath, moduleName string) *tls.Config {
	certs, err := auth.LoadControllerTrustForTLS(configuredPath)
	if err != nil {
		var loadErr *auth.ControllerTrustLoadError
		if errors.As(err, &loadErr) {
			if os.IsNotExist(loadErr.Err) {
				logging.LogWarn(moduleName, fmt.Sprintf("controllerCert %q not found; using OS trust store", loadErr.Path))
			} else {
				logging.LogWarn(moduleName, fmt.Sprintf("controllerCert %q failed to load: %v; using OS trust store", loadErr.Path, loadErr.Err))
			}
		} else {
			logging.LogWarn(moduleName, fmt.Sprintf("controllerCert failed to load: %v; using OS trust store", err))
		}
	}
	return auth.BuildControllerDialTLSConfig(secureMode, configuredPath, certs, err)
}
