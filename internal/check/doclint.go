package check

import (
	"fmt"
	"strings"
)

// DocRules says what a document of one kind must look like.
type DocRules struct {
	Sections []string // level-2 headings that must appear, in any order
	MaxLines int      // 0 means no limit
	Front    bool     // the document starts with a front matter block
}

// DocViolation is one broken rule of a document.
type DocViolation struct {
	Line int // 1-based; 0 when the rule concerns the whole document
	Msg  string
}

func (v DocViolation) String() string {
	if v.Line == 0 {
		return v.Msg
	}
	return fmt.Sprintf("line %d: %s", v.Line, v.Msg)
}

// DocLint checks the structure of a Markdown document: front matter, required sections,
// length, and that every code fence is closed (an open fence hides the rest of the page).
func DocLint(text string, r DocRules) []DocViolation {
	var out []DocViolation
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if r.Front && (len(lines) == 0 || lines[0] != "---") {
		out = append(out, DocViolation{1, "the document must start with a front matter block"})
	}
	if r.MaxLines > 0 && len(lines) > r.MaxLines {
		out = append(out, DocViolation{0, fmt.Sprintf("the document has %d lines, the limit is %d", len(lines), r.MaxLines)})
	}
	have := map[string]bool{}
	fenceAt := 0
	for i, l := range lines {
		if strings.HasPrefix(l, "```") {
			if fenceAt == 0 {
				fenceAt = i + 1
			} else {
				fenceAt = 0
			}
			continue
		}
		if fenceAt == 0 && strings.HasPrefix(l, "## ") {
			have[strings.TrimSpace(l[3:])] = true
		}
	}
	if fenceAt != 0 {
		out = append(out, DocViolation{fenceAt, "this code fence is never closed"})
	}
	for _, s := range r.Sections {
		if !have[s] {
			out = append(out, DocViolation{0, fmt.Sprintf("missing section %q", s)})
		}
	}
	return out
}
