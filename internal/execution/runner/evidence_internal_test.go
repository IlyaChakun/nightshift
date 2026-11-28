package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// T1: an acceptance job keeps its screenshots and video as artifacts under neutral names, and
// nothing else: no other file, nothing under .git, no symlink, nothing over the size limit
// (plan P5b).
func TestAcceptanceEvidenceIsKeptUnderNeutralNames(t *testing.T) {
	ws, outside := t.TempDir(), t.TempDir()
	files := map[string]int{
		"shots/the-export-holds-exactly-10001-rows.png": 3, "shots/02.JPG": 3, "video/run.webm": 3,
		"notes.txt": 3, ".git/x.png": 3, "big.png": maxEvidenceBytes + 1,
	}
	for p, n := range files {
		full := filepath.Join(ws, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, n), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(ws, "link.png")); err != nil {
		t.Fatal(err)
	}
	r := &Runner{ArtifactDir: t.TempDir()}
	claim := jobsapi.ClaimResponse{JobID: "7", JobToken: "t"}
	out, err := r.withEvidence(claim, ws, jobsapi.CompleteRequest{Artifacts: []jobsapi.Artifact{}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range out.Artifacts {
		got = append(got, a.Path)
		if _, err := os.Stat(filepath.Join(r.ArtifactDir, "7", a.Path)); err != nil {
			t.Fatal(err)
		}
	}
	at := attemptOf(claim)
	want := at + "/evidence/01.jpg " + at + "/evidence/02.png " + at + "/evidence/03.webm"
	if strings.Join(got, " ") != want {
		t.Fatalf("artifacts %v, want %s", got, want)
	}
}

func TestOnlyAcceptanceJobsKeepEvidence(t *testing.T) {
	for in, want := range map[string]bool{`{"mode":"acceptance"}`: true, `{"mode":"review"}`: false, ``: false} {
		if acceptanceJob(jobsapi.Job{Inputs: json.RawMessage(in)}) != want {
			t.Errorf("inputs %q: want %v", in, want)
		}
	}
}
