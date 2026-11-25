package main

import (
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/onboard"
)

// configCmd handles `nightshift config validate`: the whole configuration of an instance
// and a project checkout, checked before the service is restarted or a PR is opened. It
// reads files only, so it needs no secrets and no network, and runs on a laptop.
func configCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "validate" {
		_, _ = fmt.Fprintln(stderr, "nightshift config: usage: config validate [--config <dir>] [--repo-dir <checkout>] [--tenant <name>] [--stacks <dir>]")
		return exitUsage
	}
	fs := flag.NewFlagSet("config validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml, providers and tenants/; skipped when only --repo-dir is given")
	repoDir := fs.String("repo-dir", "", "checkout of a project: validate its .nightshift/ files")
	tenant := fs.String("tenant", "", "tenant name: it must exist in the instance and match the project's name")
	stacksDir := fs.String("stacks", "stacks", "the factory's stack packs; a missing directory means none")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	configSet := false
	fs.Visit(func(f *flag.Flag) { configSet = configSet || f.Name == "config" })
	v := config.NewValidator()
	failed := false
	report := func(line string, bad bool) {
		failed = failed || bad
		_, _ = fmt.Fprintln(stdout, line)
	}
	if configSet || *repoDir == "" {
		inst, err := config.LoadInstance(v, *dir)
		switch {
		case err != nil:
			report("error: "+*dir+": "+flatten(err), true)
		case *tenant != "" && !hasTenant(inst, *tenant):
			report(fmt.Sprintf("error: tenant %q is not in instance %s (it has: %v)", *tenant, inst.Name, slices.Sorted(maps.Keys(inst.Tenants))), true)
		default:
			report(fmt.Sprintf("ok: instance %s, tenants %v", inst.Name, slices.Sorted(maps.Keys(inst.Tenants))), false)
		}
	}
	if *repoDir != "" {
		packs, err := packNames(*stacksDir, *repoDir)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "nightshift config validate:", err)
			return exitFailure
		}
		problems := onboard.ValidateProject(os.DirFS(*repoDir), v, packs, *tenant)
		for _, p := range problems {
			report(p.String(), !p.Warning)
		}
		if !onboard.HasErrors(problems) {
			report("ok: "+*repoDir+"/.nightshift", false)
		}
	}
	if failed {
		return exitFailure
	}
	return exitOK
}

func hasTenant(inst *config.Instance, name string) bool {
	_, ok := inst.Tenants[name]
	return ok
}

// packNames lists the factory's stack packs and the project's own.
func packNames(factoryDir, repoDir string) ([]string, error) {
	var names []string
	for _, d := range []string{factoryDir, filepath.Join(repoDir, ".nightshift", "stacks")} {
		packs, err := loadStacks(d)
		if err != nil {
			return nil, err
		}
		names = append(names, slices.Collect(maps.Keys(packs))...)
	}
	return names, nil
}

func flatten(err error) string {
	return fmt.Sprint(err)
}
