//go:build linux

package iofog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// buildExtraHostsWithIoFog returns extraHosts with "iofog:hostIP" prepended for non-host-network
// containers, unless the user already has an iofog entry. Matches Java DockerUtil.createContainer.
func buildExtraHostsWithIoFog(extraHosts []string, hostIP string) []string {
	hasIoFog := false
	for _, h := range extraHosts {
		if strings.Contains(strings.TrimSpace(h), "iofog") {
			hasIoFog = true
			break
		}
	}
	if hostIP != "" && !hasIoFog {
		return append([]string{"iofog:" + hostIP}, extraHosts...)
	}
	return extraHosts
}

// buildHostsFile writes a per-container /etc/hosts file to targetPath.
// It injects service.local → routerIP (if non-empty) and any caller-supplied extraHosts.
func buildHostsFile(targetPath string, extraHosts []string, routerIP string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("mkdir hosts dir: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("127.0.0.1\tlocalhost\n")
	sb.WriteString("::1\t\tlocalhost ip6-localhost ip6-loopback\n")
	sb.WriteString("fe00::0\tip6-localnet\n")
	sb.WriteString("ff00::0\tip6-mcastprefix\n")
	sb.WriteString("ff02::1\tip6-allnodes\n")
	sb.WriteString("ff02::2\tip6-allrouters\n")

	if routerIP != "" {
		sb.WriteString(fmt.Sprintf("%s\tservice.local\n", routerIP))
	}

	for _, h := range extraHosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		idx := strings.Index(h, ":")
		if idx <= 0 || idx >= len(h)-1 {
			continue
		}
		ip := strings.TrimSpace(h[idx+1:])
		host := strings.TrimSpace(h[:idx])
		if ip != "" && host != "" {
			sb.WriteString(fmt.Sprintf("%s\t%s\n", ip, host))
		}
	}

	return os.WriteFile(targetPath, []byte(sb.String()), 0644)
}
