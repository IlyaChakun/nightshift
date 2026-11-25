package check

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ilyachakun/nightshift/internal/rubric"
)

// RubricFindings runs the L4 rubrics of each document kind over the worktree's documents and
// returns one finding per failed rubric (design section 8). cards returns the cards of a
// kind. A rubric that asks for context (for example the ADRs) gets the documents of those
// kinds. Pass and unknown answers give no finding, so a quiet judge never blocks a task.
func RubricFindings(ctx context.Context, dir string, kinds []DocKind, cards func(kind string) ([]rubric.Card, error), j rubric.Judge) ([]Finding, error) {
	docs, err := readDocs(dir, kinds)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, k := range kinds {
		cs, err := cards(k.Name)
		if err != nil {
			return nil, err
		}
		for _, d := range docs[k.Name] {
			fs, err := judgeDoc(ctx, j, cs, d, docs)
			if err != nil {
				return nil, err
			}
			out = append(out, fs...)
		}
	}
	return out, nil
}

// rubricFinding builds the finding. The fingerprint covers the rule and the document only,
// so a reworded rationale does not make an old finding look new (design section 8, "L4 repeats").
func rubricFinding(r rubric.Result) Finding {
	f := Finding{
		RuleID: r.RuleID, Category: "docs", Severity: Severity(r.Severity), LayerCaught: "L4",
		Message: r.CardTitle + ": " + r.Message, Location: &Location{Path: r.DocPath},
	}
	f.Fingerprint = fingerprint(f, nil)
	if r.Rationale != "" {
		f.Message += " Judge: " + strings.TrimSpace(r.Rationale)
	}
	return f
}

// readDocs reads the documents of each kind, keyed by kind name.
func readDocs(dir string, kinds []DocKind) (map[string][]rubric.Doc, error) {
	docs := map[string][]rubric.Doc{}
	for _, k := range kinds {
		files, err := filepath.Glob(filepath.Join(dir, k.Glob))
		if err != nil {
			return nil, fmt.Errorf("check: rubric %s: %w", k.Name, err)
		}
		for _, file := range files {
			raw, err := os.ReadFile(file) //nolint:gosec // G304: the path comes from a glob of the worktree
			if err != nil {
				return nil, fmt.Errorf("check: rubric: %w", err)
			}
			rel, _ := filepath.Rel(dir, file)
			docs[k.Name] = append(docs[k.Name], rubric.Doc{Path: filepath.ToSlash(rel), Text: string(raw)})
		}
	}
	return docs, nil
}

// judgeDoc asks each card's question about one document, giving a card the documents of the
// kinds it names as context.
func judgeDoc(ctx context.Context, j rubric.Judge, cards []rubric.Card, d rubric.Doc, docs map[string][]rubric.Doc) ([]Finding, error) {
	var out []Finding
	for _, c := range cards {
		var others []rubric.Doc
		for _, ck := range c.Context {
			others = append(others, docs[ck]...)
		}
		rs, err := rubric.Run(ctx, j, []rubric.Card{c}, d, others)
		if err != nil {
			return nil, fmt.Errorf("check: %w", err)
		}
		for _, r := range rs {
			out = append(out, rubricFinding(r))
		}
	}
	return out, nil
}
