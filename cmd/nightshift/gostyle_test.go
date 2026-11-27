package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The tenant Go style page describes the lint template that `nightshift standards sync` copies into
// a project, not Nightshift's own configuration. A tenant agent that reads "enforced by X" must find
// X enabled in its repository.

// standardLinters is what `linters.default: standard` enables in golangci-lint v2, as the
// template's own comment lists it.
var standardLinters = []string{"errcheck", "govet", "ineffassign", "staticcheck", "unused"}

// styleTableLinters returns the backticked names in the second column of the table under the
// "## Enforced by tools" heading.
func styleTableLinters(page string) []string {
	_, section, _ := strings.Cut(page, "## Enforced by tools")
	section, _, _ = strings.Cut(section, "\n## ")
	tick := regexp.MustCompile("`([^`]+)`")
	var names []string
	for line := range strings.SplitSeq(section, "\n") {
		cells := strings.Split(strings.ReplaceAll(line, `\|`, ""), "|")
		if len(cells) < 4 || strings.HasPrefix(strings.TrimSpace(cells[1]), "---") {
			continue
		}
		for _, m := range tick.FindAllStringSubmatch(cells[2], -1) {
			names = append(names, m[1])
		}
	}
	return names
}

// lintMismatch lists each linter or formatter the page names that the template does not enable,
// and each one the template enables that the page does not name.
func lintMismatch(page string, template []byte) ([]string, error) {
	var cfg struct {
		Linters struct {
			Default string   `yaml:"default"`
			Enable  []string `yaml:"enable"`
			Disable []string `yaml:"disable"`
		} `yaml:"linters"`
		Formatters struct {
			Enable []string `yaml:"enable"`
		} `yaml:"formatters"`
	}
	if err := yaml.Unmarshal(template, &cfg); err != nil {
		return nil, err
	}
	enabled := slices.Concat(cfg.Linters.Enable, cfg.Formatters.Enable)
	switch cfg.Linters.Default {
	case "standard", "":
		enabled = append(enabled, standardLinters...)
	case "none":
	default:
		return nil, fmt.Errorf("linters.default %q: the test knows only standard and none", cfg.Linters.Default)
	}
	enabled = slices.DeleteFunc(enabled, func(n string) bool { return slices.Contains(cfg.Linters.Disable, n) })
	named := styleTableLinters(page)
	var problems []string
	for _, n := range named {
		if !slices.Contains(enabled, n) {
			problems = append(problems, "style.md names "+n+", which the template does not enable")
		}
	}
	for _, e := range enabled {
		if !slices.Contains(named, e) {
			problems = append(problems, "the template enables "+e+", which style.md does not name")
		}
	}
	return problems, nil
}

func TestGoStyleTableMatchesTenantTemplate(t *testing.T) {
	page := readRepoFile(t, "../../standards/industry/go/style.md")
	template := readRepoFile(t, "../../standards/industry/go/files/.golangci.yml")
	problems, err := lintMismatch(page, []byte(template))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

func TestLintMismatch(t *testing.T) {
	template := []byte("linters:\n  default: standard\n  enable: [gosec]\nformatters:\n  enable: [gofmt]\n")
	t.Run("a disabled standard linter is not enabled", func(t *testing.T) {
		got, err := lintMismatch("## Enforced by tools\n\n| a | `errcheck` |\n", []byte("linters:\n  default: standard\n  disable: [errcheck]\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(got, "style.md names errcheck, which the template does not enable") {
			t.Errorf("got %q, want errcheck reported", got)
		}
	})
	t.Run("an unknown default fails", func(t *testing.T) {
		if _, err := lintMismatch("", []byte("linters:\n  default: all\n")); err == nil {
			t.Error("got no error for default: all")
		}
	})
	page := func(names string) string {
		return "# Go style\n\n## Enforced by tools\n\n| Rule | Linters |\n|---|---|\n| a rule | " + names + " |\n\n## Other\n\n| x | `revive` |\n"
	}
	tests := []struct {
		name  string
		names string
		want  []string
	}{
		{"matches", "`gosec`, `gofmt`, `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`", nil},
		{"names a linter the template lacks", "`gosec`, `gofmt`, `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `errchkjson`",
			[]string{"style.md names errchkjson, which the template does not enable"}},
		{"misses an enabled linter", "`gofmt`, `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`",
			[]string{"the template enables gosec, which style.md does not name"}},
		{"names a command, not a linter", "`gosec`, `gofmt`, `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `golangci-lint fmt`",
			[]string{"style.md names golangci-lint fmt, which the template does not enable"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lintMismatch(page(tt.names), template)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
