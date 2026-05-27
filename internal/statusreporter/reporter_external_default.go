//go:build !lite && !full

package statusreporter

var listExternalRuntimesForStatus = func(_ string) ([]string, error) {
	return nil, nil
}

var listCatalogRuntimesForStatus = func() []string {
	return nil
}
