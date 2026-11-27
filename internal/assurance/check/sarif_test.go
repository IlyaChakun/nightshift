package check_test

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/check"
)

const log = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"GoSec"}},"results":[
 {"ruleId":"G101","level":"error","message":{"text":"hardcoded credential"},
  "locations":[{"physicalLocation":{"artifactLocation":{"uri":"a/b.go"},"region":{"startLine":7,"endLine":8}}}]},
 {"ruleId":"","level":"warning","message":{"text":"odd"}}]}]}`

func TestParseSARIF(t *testing.T) {
	got, err := check.ParseSARIF(strings.NewReader(log), "security", "L1")
	if err != nil || len(got) != 2 {
		t.Fatalf("parse: %+v, %v", got, err)
	}
	first, second := got[0], got[1]
	if first.RuleID != "gosec.g101" || first.Severity != check.Block || first.Location.Path != "a/b.go" || first.Location.StartLine != 7 {
		t.Errorf("first: %+v", first)
	}
	if second.RuleID != "new" || second.Severity != check.Warn || second.Location != nil {
		t.Errorf("second: %+v", second)
	}
	if !check.Blocking(got) || check.Blocking(got[1:]) {
		t.Error("only the error blocks")
	}
}

func TestParseSARIFRejects(t *testing.T) {
	for name, in := range map[string]string{
		"not json":      `nope`,
		"wrong version": `{"version":"1.0.0","runs":[]}`,
	} {
		if _, err := check.ParseSARIF(strings.NewReader(in), "code", "L1"); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestFingerprintIgnoresLines(t *testing.T) {
	moved := strings.Replace(log, `"startLine":7,"endLine":8`, `"startLine":70,"endLine":71`, 1)
	a, _ := check.ParseSARIF(strings.NewReader(log), "code", "L1")
	b, _ := check.ParseSARIF(strings.NewReader(moved), "code", "L1")
	if a[0].Fingerprint != b[0].Fingerprint || a[0].Fingerprint == a[1].Fingerprint {
		t.Errorf("fingerprints: %q %q %q", a[0].Fingerprint, b[0].Fingerprint, a[1].Fingerprint)
	}
}

func TestNewViolations(t *testing.T) {
	f := func(fp string) check.Finding { return check.Finding{Fingerprint: fp} }
	base := []check.Finding{f("a"), f("b")}
	cur := []check.Finding{f("a"), f("a"), f("c")}
	got := check.NewViolations(base, cur)
	if len(got) != 2 || got[0].Fingerprint != "a" || got[1].Fingerprint != "c" {
		t.Errorf("new violations: %+v", got)
	}
	if got := check.NewViolations(nil, nil); got != nil {
		t.Errorf("empty: %+v", got)
	}
}

func FuzzParseSARIF(f *testing.F) {
	f.Add(log)
	f.Add(`{"version":"2.1.0","runs":[{"results":[{}]}]}`)
	f.Fuzz(func(t *testing.T, in string) {
		fs, err := check.ParseSARIF(strings.NewReader(in), "code", "L1")
		if err != nil {
			return
		}
		for _, x := range fs {
			if x.RuleID == "" || x.Message == "" || len(x.Message) > 2000 || x.Fingerprint == "" {
				t.Fatalf("invalid finding: %+v", x)
			}
		}
	})
}
