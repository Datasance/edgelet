//go:build lite

package buildmeta

import "fmt"

func init() {
	if !IsLite() {
		panic(fmt.Sprintf("build tag lite requires -X github.com/datasance/edgelet/internal/buildmeta.Flavor=lite, got %q", Flavor))
	}
}
