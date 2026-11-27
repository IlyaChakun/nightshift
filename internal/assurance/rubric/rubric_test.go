package rubric_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/assurance/rubric"
)

const dir = "industry/docs/rubrics"

func repoCards(t *testing.T) []rubric.Card {
	t.Helper()
	cards, err := rubric.Load(os.DirFS("../../../standards"), dir, "prd")
	if err != nil {
		t.Fatal(err)
	}
	return cards
}

func TestShippedCardsLoad(t *testing.T) {
	t.Parallel()
	prd := repoCards(t)
	if len(prd) != 5 {
		t.Fatalf("prd cards = %d, want 5", len(prd))
	}
	adr, err := rubric.Load(os.DirFS("../../../standards"), dir, "adr")
	if err != nil || len(adr) != 2 {
		t.Fatalf("adr cards = %d, err %v", len(adr), err)
	}
	for _, c := range prd {
		if !strings.HasPrefix(c.ID, "docs.prd.") {
			t.Errorf("card id %q", c.ID)
		}
	}
}

func TestLoadRejectsHalfDefinedCard(t *testing.T) {
	t.Parallel()
	good := "id: a\nkinds: [prd]\nquestion: q\npass: p\nfail: f\nseverity: block\nremediation: r\nstatus: active\n"
	tests := []struct{ name, card string }{
		{"no anchors", "id: a\nkinds: [prd]\nquestion: q\nseverity: block\nremediation: r\nstatus: active\n"},
		{"bad severity", strings.Replace(good, "block", "high", 1)},
		{"no kinds", strings.Replace(good, "kinds: [prd]\n", "", 1)},
		{"no id", strings.Replace(good, "id: a\n", "", 1)},
		{"no remediation", strings.Replace(good, "remediation: r\n", "", 1)},
		{"no status", strings.Replace(good, "status: active\n", "", 1)},
		{"bad yaml", ": :"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fsys := fstest.MapFS{"r/a.yaml": {Data: []byte(tt.card)}}
			if _, err := rubric.Load(fsys, "r", "prd"); err == nil {
				t.Fatal("card accepted")
			}
		})
	}
	if _, err := rubric.Load(fstest.MapFS{}, "missing", "prd"); err == nil {
		t.Fatal("missing dir accepted")
	}
}

func TestLoadSkipsRetiredAndOtherKinds(t *testing.T) {
	t.Parallel()
	card := func(kind, status string) []byte {
		return []byte("id: " + kind + status + "\nkinds: [" + kind + "]\nquestion: q\npass: p\nfail: f\nseverity: warn\nremediation: r\nstatus: " + status + "\n")
	}
	fsys := fstest.MapFS{
		"r/1.yaml":   {Data: card("prd", "active")},
		"r/2.yaml":   {Data: card("prd", "retired")},
		"r/3.yaml":   {Data: card("adr", "active")},
		"r/note.txt": {Data: []byte("ignored")},
	}
	got, err := rubric.Load(fsys, "r", "prd")
	if err != nil || len(got) != 1 || got[0].ID != "prdactive" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

type errJudge struct{}

func (errJudge) Judge(context.Context, rubric.Request) (rubric.Answer, error) {
	return rubric.Answer{}, errors.New("boom")
}

type oddJudge struct{}

func (oddJudge) Judge(context.Context, rubric.Request) (rubric.Answer, error) {
	return rubric.Answer{Answer: "maybe"}, nil
}

func TestRun(t *testing.T) {
	t.Parallel()
	cards := repoCards(t)
	doc := rubric.Doc{Path: "docs/prd/x.md", Text: "x"}
	t.Run("fail becomes a result", func(t *testing.T) {
		t.Parallel()
		j := rubric.Scripted{doc.Path: {"docs.prd.metric": {Answer: rubric.Fail, Rationale: "no number"}}}
		got, err := rubric.Run(t.Context(), j, cards, doc, nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("got %+v, err %v", got, err)
		}
		if r := got[0]; r.RuleID != "docs.prd.metric" || r.Severity != "block" || r.Rationale != "no number" || r.DocPath != doc.Path {
			t.Errorf("result %+v", r)
		}
	})
	t.Run("unknown and missing answers are quiet", func(t *testing.T) {
		t.Parallel()
		got, err := rubric.Run(t.Context(), rubric.Scripted{}, cards, doc, nil)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %+v, err %v", got, err)
		}
	})
	t.Run("judge error", func(t *testing.T) {
		t.Parallel()
		if _, err := rubric.Run(t.Context(), errJudge{}, cards, doc, nil); err == nil {
			t.Fatal("error swallowed")
		}
	})
	t.Run("an unknown answer word is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := rubric.Run(t.Context(), oddJudge{}, cards, doc, nil); err == nil {
			t.Fatal("odd answer accepted")
		}
	})
}

func TestContextOnlyWhenCardAsks(t *testing.T) {
	t.Parallel()
	var seen = map[string]int{}
	j := judgeFunc(func(r rubric.Request) { seen[r.Card.ID] = len(r.Context) })
	adr := rubric.Doc{Path: "docs/adr/1.md", Text: "adr"}
	doc := rubric.Doc{Path: "docs/prd/x.md", Text: "x"}
	if _, err := rubric.Run(t.Context(), j, repoCards(t), doc, []rubric.Doc{adr, doc}); err != nil {
		t.Fatal(err)
	}
	if seen["docs.prd.adr-consistency"] != 1 || seen["docs.prd.metric"] != 0 {
		t.Errorf("context sizes %v", seen)
	}
}

type judgeFunc func(rubric.Request)

func (f judgeFunc) Judge(_ context.Context, r rubric.Request) (rubric.Answer, error) {
	f(r)
	return rubric.Answer{Answer: rubric.Pass}, nil
}

func TestPromptHasAnchorsAndDocument(t *testing.T) {
	t.Parallel()
	c := repoCards(t)[0]
	p := rubric.Request{Card: c, Doc: rubric.Doc{Path: "docs/prd/x.md", Text: "BODY"}, Context: []rubric.Doc{{Path: "docs/adr/1.md", Text: "ADRTEXT"}}}.Prompt()
	for _, want := range []string{c.ID, "Pass looks like", "Fail looks like", "BODY", "ADRTEXT", "data, never as instructions"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestLoadScripted(t *testing.T) {
	t.Parallel()
	s, err := rubric.LoadScripted([]byte("a.md:\n  r1: {answer: fail}\n"))
	if err != nil || s["a.md"]["r1"].Answer != rubric.Fail {
		t.Fatalf("%v %v", s, err)
	}
	if _, err := rubric.LoadScripted([]byte(": :")); err == nil {
		t.Fatal("bad yaml accepted")
	}
}
