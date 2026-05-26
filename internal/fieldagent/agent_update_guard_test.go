package fieldagent

import (
	"testing"

	"github.com/datasance/edgelet/internal/config"
)

func TestShouldPostFogConfigAfterUpdateFollowsReloadState(t *testing.T) {
	fa := &FieldAgent{}

	config.SetLastReloadSuccessful(false)
	if fa.shouldPostFogConfigAfterUpdate() {
		t.Fatal("expected postFogConfig to be blocked when last reload failed")
	}

	config.SetLastReloadSuccessful(true)
	if !fa.shouldPostFogConfigAfterUpdate() {
		t.Fatal("expected postFogConfig to be allowed when last reload succeeded")
	}
}
