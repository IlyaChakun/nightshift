package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/onboard"
	"github.com/ilyachakun/nightshift/internal/stack"
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
		// The checkout is untrusted: a root refuses every path, symlinks included, that
		// leaves it, so a link cannot make the command read or probe a file outside.
		root, err := os.OpenRoot(*repoDir)
		if err != nil {
			report("error: "+*repoDir+": "+flatten(err), true)
			return exitFailure
		}
		defer func() { _ = root.Close() }()
		repo := root.FS()
		packs, err := packNames(*stacksDir, repo)
		if err != nil {
			report("error: "+*repoDir+"/.nightshift/stacks: "+flatten(err), true)
			return exitFailure
		}
		problems := onboard.ValidateProject(repo, v, packs, *tenant)
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
func packNames(factoryDir string, repo fs.FS) ([]string, error) {
	packs, err := loadStacks(factoryDir)
	if err != nil {
		return nil, err
	}
	names := slices.Collect(maps.Keys(packs))
	own, err := fs.Sub(repo, ".nightshift/stacks")
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(own, "."); err != nil { // the project has no packs of its own
		return names, nil //nolint:nilerr // a missing directory is the usual case
	}
	mine, err := stack.Load(own)
	if err != nil {
		return nil, err
	}
	return append(names, slices.Collect(maps.Keys(mine))...), nil
}

// flatten puts an error on one line, so every finding is one line of output.
func flatten(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}
