// Package airgap is the pinned image list shared by online pull and the airgap archive.
package airgap

import (
	"bufio"
	_ "embed"
	"strings"
)

//go:embed image-list.txt
var imageList string

// Images returns the pinned image references, one per line.
func Images() []string {
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(imageList))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
