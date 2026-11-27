package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/standards"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// TestChecklistPathsAreInTheWorker: every "details:" page the L0 checklist names exists in this
// repository's standards/ and sits where the worker image copies it, so an agent that follows
// the pointer finds the file (design section 9, layer L0).
func TestChecklistPathsAreInTheWorker(t *testing.T) {
	stacks, err := os.ReadDir("../../standards/industry")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range stacks {
		names = append(names, s.Name())
	}
	lines, err := standards.Checklist(os.DirFS("../../standards"), names, jobsapi.StandardsDir)
	if err != nil {
		t.Fatal(err)
	}
	details := regexp.MustCompile(`details: (\S+)$`)
	found := 0
	for _, line := range lines {
		m := details.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		found++
		rel, ok := strings.CutPrefix(m[1], jobsapi.StandardsDir+"/")
		if !ok {
			t.Errorf("%q points outside %s", line, jobsapi.StandardsDir)
			continue
		}
		if _, err := os.Stat("../../standards/" + rel); err != nil {
			t.Errorf("%q: %v", line, err)
		}
	}
	if found == 0 {
		t.Fatal("no checklist line names a page; the test checks nothing")
	}
	dockerfile := readRepoFile(t, "../../Dockerfile")
	_, worker, _ := strings.Cut(dockerfile, " AS worker\n")
	worker, _, _ = strings.Cut(worker, "\nFROM ")
	if !strings.Contains(worker, "COPY standards "+jobsapi.StandardsDir+"\n") {
		t.Errorf("the worker stage of the Dockerfile must copy standards to %s", jobsapi.StandardsDir)
	}
}
