//go:build linux && cgo

package statusreporter

import edgeletcontainerdd "github.com/datasance/edgelet/pkg/containerd"

var listExternalRuntimesForStatus = func(_ string) ([]string, error) {
	return nil, nil
}

var listCatalogRuntimesForStatus = func() []string {
	catalog := edgeletcontainerdd.BuildRuntimeCatalog()
	if len(catalog) == 0 {
		return nil
	}
	handlers := make([]string, 0, len(catalog))
	for _, entry := range catalog {
		if entry.Handler != "" {
			handlers = append(handlers, entry.Handler)
		}
	}
	return sortedUniqueStrings(handlers)
}
