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
		return deploytest.Target{Deployer: d, Tenant: "example-solo", Revisions: [3]deployapi.Revision{"r1", "r2", "r3"}, FailRollout: d.FailRollout}
	})
}
