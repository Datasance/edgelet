//go:build linux

package edgelet

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// buildHostsFile writes a per-container /etc/hosts file to targetPath.
// It includes baseline localhost entries and validated caller-supplied extraHosts only.
func buildHostsFile(targetPath string, extraHosts []string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0750); err != nil {
		return fmt.Errorf("mkdir hosts dir: %w", err)
	}

	var sb strings.Builder
	_, _ = sb.WriteString("127.0.0.1\tlocalhost\n")
	_, _ = sb.WriteString("::1\t\tlocalhost ip6-localhost ip6-loopback\n")
	_, _ = sb.WriteString("fe00::0\tip6-localnet\n")
	_, _ = sb.WriteString("ff00::0\tip6-mcastprefix\n")
	_, _ = sb.WriteString("ff02::1\tip6-allnodes\n")
	_, _ = sb.WriteString("ff02::2\tip6-allrouters\n")

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
			_, _ = fmt.Fprintf(&sb, "%s\t%s\n", ip, host)
		}
	}

	return os.WriteFile(targetPath, []byte(sb.String()), 0o644) // #nosec G306 -- container /etc/hosts must be world-readable
}

// buildResolvConfFile writes a per-container /etc/resolv.conf file pointing at
// the embedded bridge-scoped DNS server.
func buildResolvConfFile(targetPath string, nameserver string) error {
	if strings.TrimSpace(nameserver) == "" {
		return errors.New("nameserver is required")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0750); err != nil {
		return fmt.Errorf("mkdir resolv dir: %w", err)
	}
	content := "nameserver " + strings.TrimSpace(nameserver) + "\nsearch svc.bridge.local\noptions ndots:0\n"
	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil { // #nosec G306 -- container /etc/resolv.conf must be world-readable
		return fmt.Errorf("write resolv.conf: %w", err)
	}
	return nil
}
