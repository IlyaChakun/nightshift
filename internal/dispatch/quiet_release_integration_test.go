package dispatch_test

import (
	"context"
	"testing"
	"time"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/config"
)

// T4, design section 17: a release waits for the end of quiet hours, and a tenant that releases
// nothing merges at any hour.
func TestAReleaseWaitsForTheEndOfQuietHours(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withRelease(deployfake.New())
	r.tenant.Schedule = config.Schedule{Timezone: "Europe/Zurich", QuietHours: "22:00-09:00"}
	r.c.Now = func() time.Time { return time.Date(2026, 11, 26, 22, 0, 0, 0, time.UTC) } // 23:00 in Zurich
	r.toReady()
	r.advance() // ready -> merging; the train must not run
	if pr, _ := r.twin.PR("example/app", 1); pr.Merged {
		t.Fatal("the pull request was merged in quiet hours although a release follows")
	}
	r.advance()
	if pr, _ := r.twin.PR("example/app", 1); pr.Merged {
		t.Fatal("a second tick at night merged the pull request")
	}
	r.c.Now = func() time.Time { return time.Date(2026, 11, 27, 8, 5, 0, 0, time.UTC) } // 09:05 in Zurich
	r.advance()
	if pr, _ := r.twin.PR("example/app", 1); !pr.Merged {
		t.Fatalf("the pull request was not merged after the quiet hours; task is %s", r.task.State)
	}
}

func TestAReleaseGoesAtNightWhenTheTenantAllowsIt(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withRelease(deployfake.New())
	r.tenant.Schedule = config.Schedule{Timezone: "UTC", QuietHours: "22:00-09:00", ReleasesInQuietHours: true}
	r.c.Now = func() time.Time { return time.Date(2026, 11, 26, 23, 0, 0, 0, time.UTC) }
	r.toReady()
	r.advance()
	if pr, _ := r.twin.PR("example/app", 1); !pr.Merged {
		t.Fatal("releases_in_quiet_hours did not let the release go")
	}
}

func TestATenantWithoutReleaseMergesAtNight(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.tenant.Schedule = config.Schedule{Timezone: "UTC", QuietHours: "22:00-09:00"}
	r.c.Now = func() time.Time { return time.Date(2026, 11, 26, 23, 0, 0, 0, time.UTC) }
	r.toReady()
	r.advance()
	if pr, _ := r.twin.PR("example/app", 1); !pr.Merged {
		t.Fatal("a tenant without a release capability must merge at night")
	}
}

// A deployer is wired for every tenant, so Release is set even when the repository declares no
// release target; the capability, not the hook, decides (regression of a review finding).
func TestATenantWhoseRepositoryHasNoReleaseTargetMergesAtNightWithADeployerWired(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withRelease(deployfake.New())
	r.c.Train.Releases = func(context.Context, string) (bool, error) { return false, nil }
	r.tenant.Schedule = config.Schedule{Timezone: "UTC", QuietHours: "22:00-09:00"}
	r.c.Now = func() time.Time { return time.Date(2026, 11, 26, 23, 0, 0, 0, time.UTC) }
	r.toReady()
	r.advance()
	if pr, _ := r.twin.PR("example/app", 1); !pr.Merged {
		t.Fatal("no release target: the merge must not wait for the morning")
	}
}

func TestAReleaseTargetHoldsTheMergeAtNight(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withRelease(deployfake.New())
	r.c.Train.Releases = func(context.Context, string) (bool, error) { return true, nil }
	r.tenant.Schedule = config.Schedule{Timezone: "UTC", QuietHours: "22:00-09:00"}
	r.c.Now = func() time.Time { return time.Date(2026, 11, 26, 23, 0, 0, 0, time.UTC) }
	r.toReady()
	r.advance()
	if pr, _ := r.twin.PR("example/app", 1); pr.Merged {
		t.Fatal("a release target must hold the merge at night")
	}
}
