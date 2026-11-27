package fake_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi/deploytest"
)

func TestTheTwinPassesTheDeployContract(t *testing.T) {
	t.Parallel()
	deploytest.Run(t, func(*testing.T) deploytest.Target {
		d := fake.New()
		d.Bad("bad")
		return deploytest.Target{Deployer: d, Tenant: "example-solo", Revisions: [2]deployapi.Revision{"r1", "r2"}, Bad: "bad"}
	})
}
