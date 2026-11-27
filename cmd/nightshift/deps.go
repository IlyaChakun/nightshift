package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ilyachakun/nightshift/internal/assurance/deps"
)

// depsCmd reads deps.dev about one package: `nightshift deps check <system> <name> [version]`.
// It is the tool of the build-vs-buy skill; it reports and the exit code stays zero when it
// found something, because the decision is the owner's.
func depsCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "check" {
		_, _ = fmt.Fprintln(stderr, "usage: nightshift deps check [--allow MIT,Apache-2.0] <npm|go|pypi|maven|cargo> <name> [version]")
		return exitUsage
	}
	fs := flag.NewFlagSet("deps check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	allow := fs.String("allow", strings.Join(deps.DefaultLicenses, ","), "SPDX ids that are acceptable")
	base := fs.String("base-url", deps.DefaultBase, "deps.dev API")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) < 2 || len(rest) > 3 {
		_, _ = fmt.Fprintln(stderr, "usage: nightshift deps check [--allow ...] <system> <name> [version]")
		return exitUsage
	}
	version := ""
	if len(rest) == 3 {
		version = rest[2]
	}
	r, err := deps.Client{Base: *base}.Check(context.Background(), rest[0], rest[1], version, strings.Split(*allow, ","))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift deps check: %v\n", err)
		return exitFailure
	}
	_, _ = io.WriteString(stdout, r.Format())
	return exitOK
}
