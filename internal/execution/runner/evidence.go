package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// Limits of the evidence one acceptance job keeps: enough for a card and a replay, small enough
// for the artifacts volume.
const (
	maxEvidenceFiles = 20
	maxEvidenceBytes = 25 << 20
)

// evidenceExt are the screenshots and videos an acceptance job keeps as artifacts.
var evidenceExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".webm": true, ".mp4": true}

// acceptanceJob reports whether a job is a Verifier acceptance (its inputs say mode acceptance).
func acceptanceJob(job jobsapi.Job) bool {
	var in struct {
		Mode string `json:"mode"`
	}
	return json.Unmarshal(job.Inputs, &in) == nil && in.Mode == "acceptance"
}

// withEvidence stores the screenshots and videos an acceptance job left in dir (its evidence
// directory, or the checkout for a harness without a browser tool) as
// artifacts `evidence/NN.<ext>` of the attempt (design section 14), in path order. The files get
// neutral names, because the agent chooses its own and a name may quote a hidden scenario. Only
// regular files count (no symlinks); a file over the size limit and files past the count limit
// are skipped.
func (r *Runner) withEvidence(claim jobsapi.ClaimResponse, dir string, out jobsapi.CompleteRequest) (jobsapi.CompleteRequest, error) {
	var found []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is no evidence; keep walking
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() && evidenceExt[strings.ToLower(filepath.Ext(p))] {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("runner: evidence: %w", err)
	}
	slices.Sort(found)
	n := 0
	for _, p := range found {
		if n == maxEvidenceFiles {
			break
		}
		data, ok := readSmall(p)
		if !ok {
			continue
		}
		n++
		name := fmt.Sprintf("evidence/%02d%s", n, strings.ToLower(filepath.Ext(p)))
		art, err := r.writeArtifact(claim.JobID, attemptOf(claim), name, data)
		if err != nil {
			return out, err
		}
		out.Artifacts = append(out.Artifacts, art)
	}
	return out, nil
}

// readSmall reads a file of at most maxEvidenceBytes.
func readSmall(p string) ([]byte, bool) {
	f, err := os.Open(p) //nolint:gosec // a path the walk found inside the job's workspace
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxEvidenceBytes+1))
	return data, err == nil && len(data) <= maxEvidenceBytes
}
