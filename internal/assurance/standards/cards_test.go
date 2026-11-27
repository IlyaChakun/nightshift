package standards_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/assurance/standards"
)

func card(extra string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("id: api.size\ntitle: A handler is short\nenforcement: guidance\nseverity: warn\nstatus: active\nsource: x\n" + extra)}
}

const general = "params:\n  max_lines: { type: int, stricter: lower, default: 40 }\n  style: { type: string, enum: [rest, grpc], default: rest }\n"

func resolve(t *testing.T, fsys fstest.MapFS) (standards.Resolved, error) {
	t.Helper()
	rules, err := standards.LoadRules(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return standards.Resolve(rules)
}

func TestTheStrongerLevelWinsAndNeedsAReason(t *testing.T) {
	t.Parallel()
	res, err := resolve(t, fstest.MapFS{
		"industry/go/size.yaml": card(general),
		"project/size.yaml":     card("reason: our handlers are small\nparams: { max_lines: 25 }\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rules) != 1 || res.Rules[0].Level != "project" || res.Rules[0].Effective["max_lines"] != 25 || res.Rules[0].Effective["style"] != "rest" {
		t.Fatalf("resolved: %+v", res.Rules)
	}
	if len(res.Weakened) != 0 {
		t.Fatalf("a lower limit is stricter: %v", res.Weakened)
	}
	_, err = resolve(t, fstest.MapFS{"industry/size.yaml": card(general), "project/size.yaml": card("")})
	if err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("an override without a reason: %v", err)
	}
}

func TestAWeakerParamIsReportedForTheOwner(t *testing.T) {
	t.Parallel()
	res, err := resolve(t, fstest.MapFS{
		"industry/size.yaml": card(general),
		"house/size.yaml":    card("reason: legacy code\nparams: { max_lines: 80 }\n"),
	})
	if err != nil || len(res.Weakened) != 1 || res.Weakened[0] != "api.size.max_lines" {
		t.Fatalf("weakened %v, err %v", res.Weakened, err)
	}
}

func TestParamsAreCheckedAgainstTheirDeclaration(t *testing.T) {
	t.Parallel()
	for name, override := range map[string]string{
		"wrong type":  "reason: r\nparams: { max_lines: lots }\n",
		"not in enum": "reason: r\nparams: { style: soap }\n",
		"undeclared":  "reason: r\nparams: { hue: red }\n",
	} {
		if _, err := resolve(t, fstest.MapFS{"industry/size.yaml": card(general), "project/size.yaml": card(override)}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestNothingOverridesAFactoryRule(t *testing.T) {
	t.Parallel()
	_, err := resolve(t, fstest.MapFS{"factory/size.yaml": card(""), "project/size.yaml": card("reason: r\n")})
	if err == nil || !strings.Contains(err.Error(), "factory") {
		t.Fatalf("err = %v", err)
	}
	if _, err := resolve(t, fstest.MapFS{"house/a.yaml": card(""), "house/b.yaml": card("")}); err == nil {
		t.Fatal("the same id twice at one level must be refused")
	}
}

func TestTheShippedStandardsResolve(t *testing.T) {
	t.Parallel()
	rules, err := standards.LoadRules(os.DirFS("../../../standards"))
	if err != nil || len(rules) == 0 {
		t.Fatalf("rules %d, err %v", len(rules), err)
	}
	if _, err := standards.Resolve(rules); err != nil {
		t.Fatal(err)
	}
}

func TestAMiddleLevelOfValuesDoesNotHideTheDeclaration(t *testing.T) {
	t.Parallel()
	res, err := resolve(t, fstest.MapFS{
		"industry/size.yaml": card(general),
		"house/size.yaml":    card("reason: legacy\nparams: { max_lines: 50 }\n"),
		"project/size.yaml":  card("reason: ours\nparams: { max_lines: 45 }\n"),
	})
	if err != nil || len(res.Rules) != 1 || res.Rules[0].Level != "project" || res.Rules[0].Effective["max_lines"] != 45 {
		t.Fatalf("resolved %+v, err %v", res.Rules, err)
	}
	if len(res.Weakened) != 1 || res.Weakened[0] != "api.size.max_lines" {
		t.Fatalf("weakened: %v", res.Weakened)
	}
	// A middle level without params must not make the project's params undeclared.
	if _, err := resolve(t, fstest.MapFS{
		"industry/size.yaml": card(general), "house/size.yaml": card("reason: r\n"),
		"project/size.yaml": card("reason: r\nparams: { max_lines: 30 }\n"),
	}); err != nil {
		t.Fatal(err)
	}
}
