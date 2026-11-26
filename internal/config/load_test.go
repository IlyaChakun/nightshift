package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// instanceDir builds an instance directory from testdata: instance.yaml and providers.yaml
// of the named instance, plus the given synthetic tenant packs under tenants/.
func instanceDir(t *testing.T, instance string, tenants ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"instance.yaml", "providers.yaml"} {
		copyFile(t, filepath.Join("..", "..", "testdata", "instances", instance, f), filepath.Join(dir, f))
	}
	for _, name := range tenants {
		copyFile(t, filepath.Join("..", "..", "testdata", "tenants", name, "tenant.yaml"),
			filepath.Join(dir, "tenants", name, "tenant.yaml"))
	}
	return dir
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	raw, err := os.ReadFile(from) //nolint:gosec // test fixtures
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, raw, 0o600); err != nil { //nolint:gosec // test temp dir
		t.Fatal(err)
	}
}

func TestLoadInstance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		instance, tenant, governance string
	}{
		{"personal", "example-solo", "solo"},
		{"work", "example-team", "team"},
	}
	for _, tc := range tests {
		t.Run(tc.tenant, func(t *testing.T) {
			t.Parallel()
			inst, err := LoadInstance(NewValidator(), instanceDir(t, tc.instance, tc.tenant))
			if err != nil {
				t.Fatal(err)
			}
			tenant, ok := inst.Tenants[tc.tenant]
			if !ok || tenant.Governance != tc.governance || len(tenant.People.Approvers) == 0 {
				t.Fatalf("tenant %s: %+v", tc.tenant, tenant)
			}
			if inst.Name != tc.instance || len(inst.Providers) == 0 {
				t.Fatalf("instance: %+v", inst)
			}
		})
	}
}

func TestLoadInstanceRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		prepare func(t *testing.T) string
		want    string
	}{
		{"provider from another instance", func(t *testing.T) string {
			return instanceDir(t, "personal", "example-team") // uses claude-corp and copilot
		}, `uses provider "claude-corp"`},
		{"no tenants", func(t *testing.T) string {
			dir := instanceDir(t, "personal")
			if err := os.MkdirAll(filepath.Join(dir, "tenants"), 0o750); err != nil {
				t.Fatal(err)
			}
			return dir
		}, "no tenant packs"},
		{"name differs from directory", func(t *testing.T) string {
			dir := instanceDir(t, "personal", "example-solo")
			if err := os.Rename(filepath.Join(dir, "tenants", "example-solo"), filepath.Join(dir, "tenants", "other")); err != nil {
				t.Fatal(err)
			}
			return dir
		}, "must match its directory"},
		{"invalid tenant", func(t *testing.T) string {
			dir := instanceDir(t, "personal", "example-solo")
			path := filepath.Join(dir, "tenants", "example-solo", "tenant.yaml")
			raw, _ := os.ReadFile(path) //nolint:gosec // test fixture
			raw = []byte(strings.Replace(string(raw), "governance: solo", "governance: anarchy", 1))
			if err := os.WriteFile(path, raw, 0o600); err != nil { //nolint:gosec // test temp dir
				t.Fatal(err)
			}
			return dir
		}, "governance"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadInstance(NewValidator(), tc.prepare(t))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

func TestSecrets(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, value string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("chat_bot_token", "xoxb-test-only\n", 0o400)
	write("open", "value", 0o644)
	write("empty", "\n", 0o600)
	s := Secrets{Dir: dir}

	if got, err := s.Read("chat_bot_token"); err != nil || got != "xoxb-test-only" {
		t.Errorf("Read: %q, %v", got, err)
	}
	for name, want := range map[string]string{
		"open":          "readable by others",
		"empty":         "empty secret",
		"missing":       "no such file",
		"../etc/shadow": "invalid name",
		"Upper":         "invalid name",
	} {
		if _, err := s.Read(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Read(%q): got %v, want %q", name, err, want)
		}
	}
}

func TestMonthlyPoolsDistinguishZeroFiniteAndUncapped(t *testing.T) {
	dir := instanceDir(t, "personal", "example-solo")
	path := filepath.Join(dir, "providers.yaml")
	raw, err := os.ReadFile(path) //nolint:gosec // synthetic instance fixture
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		value string
		held  bool
	}{{"0", true}, {"3", true}, {"null", false}} {
		changed := strings.Replace(string(raw), "monthly_usd: 200", "monthly_usd: "+tc.value, 1)
		if err = os.WriteFile(path, []byte(changed), 0o600); err != nil {
			t.Fatal(err)
		}
		inst, err := LoadInstance(NewValidator(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if inst.Pools.NeedsCreditReservation(inst.Providers["claude-api"]) != tc.held || inst.Pools.NeedsCreditReservation(inst.Providers["claude-sub"]) {
			t.Fatalf("wrong reservation requirement for monthly pool %s", tc.value)
		}
	}
	missing := strings.Replace(string(raw), "pool: anthropic-credits", "pool: unknown", 1)
	if err = os.WriteFile(path, []byte(missing), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInstance(NewValidator(), dir); err == nil || !strings.Contains(err.Error(), "undefined credit pool") {
		t.Fatalf("undefined pool: %v", err)
	}
}
