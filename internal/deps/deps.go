// Package deps checks a package before the factory takes it as a dependency: licence, age,
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

// DefaultLicences are the SPDX ids taken without asking; a tenant passes its own list.
var DefaultLicences = []string{"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "0BSD", "Unlicense", "MPL-2.0"}

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
	Licences              []string
	LicenceAllowed        bool
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
	for _, v := range pkg.Versions {
		if t, err := time.Parse(time.RFC3339, v.PublishedAt); err == nil && t.After(r.LastRelease) {
			r.LastRelease = t
		}
		if version == "" && v.IsDefault {
			version = v.VersionKey.Version
		}
	}
	if version == "" {
		version = pkg.Versions[len(pkg.Versions)-1].VersionKey.Version
	}
	r.Version = version
	var ver versionDoc
	base := fmt.Sprintf("/v3/systems/%s/packages/%s/versions/%s", r.System, url.PathEscape(name), url.PathEscape(version))
	if err := c.get(ctx, base, &ver); err != nil {
		return r, err
	}
	r.Licences = ver.Licenses
	r.LicenceAllowed = len(ver.Licenses) > 0 && !slices.ContainsFunc(ver.Licenses, func(l string) bool { return !slices.Contains(allowed, l) })
	r.Published, _ = time.Parse(time.RFC3339, ver.PublishedAt)
	for _, a := range ver.AdvisoryKeys {
		r.Advisories = append(r.Advisories, a.ID)
	}
	var tree depsDoc
	if err := c.get(ctx, base+":dependencies", &tree); err == nil {
		for _, n := range tree.Nodes {
			if n.Relation != "SELF" {
				r.TreeSize++
			}
		}
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
	r.findings(now())
	return r, nil
}

// Thresholds that turn a fact into a finding.
const (
	youngDays = 14
	staleDays = 730
	bigTree   = 100
)

func (r *Report) findings(now time.Time) {
	if len(r.Licences) == 0 {
		r.Findings = append(r.Findings, "no licence is recorded")
	} else if !r.LicenceAllowed {
		r.Findings = append(r.Findings, "licence "+strings.Join(r.Licences, ", ")+" is not in the allowed list")
	}
	if len(r.Advisories) > 0 {
		r.Findings = append(r.Findings, fmt.Sprintf("%d known advisories: %s", len(r.Advisories), strings.Join(r.Advisories, ", ")))
	}
	if !r.Published.IsZero() && r.AgeDays < youngDays {
		r.Findings = append(r.Findings, fmt.Sprintf("this version is %d days old", r.AgeDays))
	}
	if !r.LastRelease.IsZero() && now.Sub(r.LastRelease).Hours()/24 > staleDays {
		r.Findings = append(r.Findings, "no release for more than two years")
	}
	if r.TreeSize > bigTree {
		r.Findings = append(r.Findings, fmt.Sprintf("it brings %d transitive dependencies", r.TreeSize))
	}
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
	fmt.Fprintf(&b, "  licence: %s (allowed: %t)\n", strings.Join(r.Licences, ", "), r.LicenceAllowed)
	fmt.Fprintf(&b, "  published: %s (%d days ago); last release: %s\n", r.Published.Format("2006-01-02"), r.AgeDays, r.LastRelease.Format("2006-01-02"))
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
