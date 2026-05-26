//go:build full

package buildmeta

import "fmt"

func init() {
	if !IsFull() {
		panic(fmt.Sprintf("build tag full requires -X github.com/datasance/edgelet/internal/buildmeta.Flavor=full, got %q", Flavor))
	}
}
