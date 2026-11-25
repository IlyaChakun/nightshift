// Package deps checks a package before the factory takes it as a dependency: license, age,
// last release, known advisories and the size of its dependency tree, from deps.dev
// (design section 6, "Research before building"). It reads data; it decides nothing.
package deps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// DefaultBase is the deps.dev API.
const DefaultBase = "https://api.deps.dev"

// DefaultLicenses are the SPDX ids taken without asking; a tenant passes its own list.
var DefaultLicenses = []string{"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "0BSD", "Unlicense", "MPL-2.0"}

// maxBody bounds one API answer.
const maxBody = 4 << 20

// Client reads deps.dev.
type Client struct {
	Base string
	HTTP *http.Client
	Now  func() time.Time
}

// Report is what the check found. Findings are the facts a person should weigh.
type Report struct {
	System, Name, Version string
	Licenses              []string
	LicenseAllowed        bool
	Published             time.Time
	LastRelease           time.Time
	AgeDays               int
	Advisories            []string
	TreeSize              int
	Findings              []string
	// Unchecked lists what this check does not cover, so a clean report is not read as more.
	Unchecked []string
}

type versionKey struct {
	Version string `json:"version"`
}

type packageDoc struct {
	Versions []struct {
		VersionKey  versionKey `json:"versionKey"`
		PublishedAt string     `json:"publishedAt"`
		IsDefault   bool       `json:"isDefault"`
	} `json:"versions"`
}

type versionDoc struct {
	Licenses     []string `json:"licenses"`
	PublishedAt  string   `json:"publishedAt"`
	AdvisoryKeys []struct {
		ID string `json:"id"`
	} `json:"advisoryKeys"`
}

type depsDoc struct {
	Nodes []struct {
		Relation string `json:"relation"`
	} `json:"nodes"`
}

// Check reads deps.dev for system (npm, go, pypi, maven, cargo) and name. An empty version means the
// default version. allowed lists the SPDX ids that are acceptable.
func (c Client) Check(ctx context.Context, system, name, version string, allowed []string) (Report, error) {
	r := Report{System: strings.ToLower(system), Name: name, Unchecked: []string{"install scripts", "number of dependents", "API fit"}}
	var pkg packageDoc
	if err := c.get(ctx, fmt.Sprintf("/v3/systems/%s/packages/%s", r.System, url.PathEscape(name)), &pkg); err != nil {
		return r, err
	}
	if len(pkg.Versions) == 0 {
		return r, fmt.Errorf("deps: %s has no versions", name)
	}
	r.LastRelease, r.Version = pkg.latest(), pkg.pick(version)
	version = r.Version
	var ver versionDoc
	base := fmt.Sprintf("/v3/systems/%s/packages/%s/versions/%s", r.System, url.PathEscape(name), url.PathEscape(version))
	if err := c.get(ctx, base, &ver); err != nil {
		return r, err
	}
	r.Licenses = ver.Licenses
	r.LicenseAllowed = len(ver.Licenses) > 0 && !slices.ContainsFunc(ver.Licenses, func(l string) bool { return !slices.Contains(allowed, l) })
	r.Published, _ = time.Parse(time.RFC3339, ver.PublishedAt)
	for _, a := range ver.AdvisoryKeys {
		r.Advisories = append(r.Advisories, a.ID)
	}
	if n, err := c.treeSize(ctx, base); err == nil {
		r.TreeSize = n
	} else {
		r.Unchecked = append(r.Unchecked, "size of the dependency tree")
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	if !r.Published.IsZero() {
		r.AgeDays = int(now().Sub(r.Published).Hours() / 24)
	}
	r.Findings = r.findingsAt(now())
	return r, nil
}

// Thresholds that turn a fact into a finding.
const (
	youngDays = 14
	staleDays = 730
	bigTree   = 100
)

func (r Report) findingsAt(now time.Time) []string {
	var out []string
	if len(r.Licenses) == 0 {
		out = append(out, "no license is recorded")
	} else if !r.LicenseAllowed {
		out = append(out, "license "+strings.Join(r.Licenses, ", ")+" is not in the allowed list")
	}
	if len(r.Advisories) > 0 {
		out = append(out, fmt.Sprintf("%d known advisories: %s", len(r.Advisories), strings.Join(r.Advisories, ", ")))
	}
	if !r.Published.IsZero() && r.AgeDays < youngDays {
		out = append(out, fmt.Sprintf("this version is %d days old", r.AgeDays))
	}
	if !r.LastRelease.IsZero() && now.Sub(r.LastRelease).Hours()/24 > staleDays {
		out = append(out, "no release for more than two years")
	}
	if r.TreeSize > bigTree {
		out = append(out, fmt.Sprintf("it brings %d transitive dependencies", r.TreeSize))
	}
	return out
}

// latest is the time of the newest release.
func (p packageDoc) latest() time.Time {
	var last time.Time
	for _, v := range p.Versions {
		if t, err := time.Parse(time.RFC3339, v.PublishedAt); err == nil && t.After(last) {
			last = t
		}
	}
	return last
}

// pick returns want, or the default version, or the last one listed.
func (p packageDoc) pick(want string) string {
	if want != "" {
		return want
	}
	for _, v := range p.Versions {
		if v.IsDefault {
			return v.VersionKey.Version
		}
	}
	return p.Versions[len(p.Versions)-1].VersionKey.Version
}

func (c Client) treeSize(ctx context.Context, base string) (int, error) {
	var tree depsDoc
	if err := c.get(ctx, base+":dependencies", &tree); err != nil {
		return 0, err
	}
	n := 0
	for _, node := range tree.Nodes {
		if node.Relation != "SELF" {
			n++
		}
	}
	return n, nil
}

func (c Client) get(ctx context.Context, path string, out any) error {
	base := c.Base
	if base == "" {
		base = DefaultBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return err
	}
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := h.Do(req)
	if err != nil {
		return fmt.Errorf("deps: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deps: %s: %s", path, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out)
}

// Format renders the report for a person or an agent. Text from the registry is data and is
// printed only as facts of the report.
func (r Report) Format() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s@%s\n", r.System, r.Name, r.Version)
	fmt.Fprintf(&b, "  license: %s (allowed: %t)\n", strings.Join(r.Licenses, ", "), r.LicenseAllowed)
	fmt.Fprintf(&b, "  published: %s (%d days ago); last release: %s\n", date(r.Published), r.AgeDays, date(r.LastRelease))
	fmt.Fprintf(&b, "  advisories: %d; transitive dependencies: %d\n", len(r.Advisories), r.TreeSize)
	if len(r.Findings) == 0 {
		b.WriteString("  findings: none\n")
	}
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "  finding: %s\n", f)
	}
	fmt.Fprintf(&b, "  not checked: %s\n", strings.Join(r.Unchecked, ", "))
	return b.String()
}

func date(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Format("2006-01-02")
}
