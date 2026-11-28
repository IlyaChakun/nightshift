package dispatch

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// maxEvidenceFiles caps the files a card lists, so it stays a short chat message.
const maxEvidenceFiles = 6

// evidencePath is a screenshot or video the runner kept from an acceptance job's workspace:
// `<attempt>/evidence/NN.<ext>`. The runner names them, never the agent, so no file name on the
// card can quote a hidden scenario, and no other file of the job (logs, the scenarios it was
// given) reaches the chat.
var evidencePath = regexp.MustCompile(`^[0-9a-f]{16}/evidence/[0-9]{2}\.(png|jpe?g|webp|gif|webm|mp4)$`)

// evidenceLabels are the fixed words of the evidence card in the chat language.
type evidenceLabels struct {
	Accepted, PR, Preview, Files, Metrics, More, Review, Release string
}

func evidenceLabelsFor(ru bool) evidenceLabels {
	if ru {
		return evidenceLabels{
			Accepted: "прошла приёмку на превью", PR: "Пул-реквест", Preview: "Превью", Files: "Скриншоты и видео",
			Metrics: "Приёмка", More: "ещё", Review: "Нужно ваше ревью пул-реквеста на GitHub.", Release: "Дальше релиз.",
		}
	}
	return evidenceLabels{
		Accepted: "passed acceptance on its preview", PR: "Pull request", Preview: "Preview", Files: "Screenshots and video",
		Metrics: "Acceptance", More: "more", Review: "Your review of the pull request on GitHub is needed.", Release: "It goes to release next.",
	}
}

// postEvidenceCard queues the "Ready for acceptance" card of a task whose acceptance passed
// (design section 9, #deployments): the preview it ran on, the screenshots and video the job
// left in its artifact directory, and the job's duration and cost. It is queued with the move,
// in the same transaction, once per head. A task with no chat thread and an owner without a
// chat get no card; the pull request still holds the review.
func (c *Conductor) postEvidenceCard(ctx context.Context, q *bookdb.Queries, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob) error {
	ref, ok := ownerThread(t, task)
	if !ok || c.Outbox == nil {
		return nil
	}
	files, err := evidenceFiles(ctx, q, job)
	if err != nil {
		return err
	}
	text := evidenceText(task, job, files, evidenceLabelsFor(c.russian(ctx, task)))
	key := fmt.Sprintf("evidence:%d:%s", task.ID, task.HeadSha.String)
	return c.postCard(ctx, q, task, chatapi.CardReady, text, nil, ref, key)
}

// evidenceFiles are the screenshots and videos an acceptance job left, as card paths, each
// content once.
func evidenceFiles(ctx context.Context, q *bookdb.Queries, job bookdb.BookJob) ([]string, error) {
	arts, err := q.ListJobArtifacts(ctx, job.ID)
	if err != nil {
		return nil, fmt.Errorf("conductor: artifacts of job %d: %w", job.ID, err)
	}
	var files []string
	seen := map[string]bool{} // the same image saved twice is shown once (V4 on the Mac)
	for _, a := range arts {
		if evidencePath.MatchString(a.Path) && (a.Sha256 == "" || !seen[a.Sha256]) {
			seen[a.Sha256] = true
			files = append(files, path.Join("artifacts", strconv.FormatInt(job.ID, 10), a.Path))
		}
	}
	return files, nil
}

// evidenceText is the card's body; task is the task after its move.
func evidenceText(task bookdb.BookTask, job bookdb.BookJob, files []string, lbl evidenceLabels) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s: %s.", task.ID, chatapi.Title(task.Title), lbl.Accepted)
	if n := prNumber(task); n > 0 {
		fmt.Fprintf(&b, "\n%s: #%d", lbl.PR, n)
	}
	b.WriteString(evidenceLines(job, files, lbl))
	if m := jobMetrics(job); m != "" {
		fmt.Fprintf(&b, "\n%s: %s", lbl.Metrics, m)
	}
	if flow.State(task.State) == flow.HumanReview {
		b.WriteString("\n" + lbl.Review)
	} else {
		b.WriteString("\n" + lbl.Release)
	}
	return b.String()
}

// evidenceLines are the preview and the files of an acceptance job, each on its own line.
func evidenceLines(job bookdb.BookJob, files []string, lbl evidenceLabels) string {
	var b strings.Builder
	var in struct {
		Inputs struct {
			URL   string `json:"preview_url"`
			Local string `json:"preview_local"` // where a person on the engine's machine opens it
		} `json:"inputs"`
	}
	_ = json.Unmarshal(job.Payload, &in) // the conductor wrote it; unreadable means no URL
	if u := cmp.Or(in.Inputs.Local, in.Inputs.URL); u != "" {
		fmt.Fprintf(&b, "\n%s: %s", lbl.Preview, u)
	}
	if len(files) > 0 {
		shown := files[:min(len(files), maxEvidenceFiles)]
		fmt.Fprintf(&b, "\n%s: %s", lbl.Files, strings.Join(shown, ", "))
		if extra := len(files) - len(shown); extra > 0 {
			fmt.Fprintf(&b, " (+%d %s)", extra, lbl.More)
		}
	}
	return b.String()
}

// jobMetrics is a job's duration and cost, or what of them the book holds.
func jobMetrics(job bookdb.BookJob) string {
	var parts []string
	if job.StartedAt.Valid && job.FinishedAt.Valid {
		parts = append(parts, job.FinishedAt.Time.Sub(job.StartedAt.Time).Round(time.Second).String())
	}
	var u struct {
		CostUSD *float64 `json:"cost_usd"`
	}
	// Zero is a harness without a price (a subscription), not a free job.
	if json.Unmarshal(job.Usage, &u) == nil && u.CostUSD != nil && *u.CostUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", *u.CostUSD))
	}
	return strings.Join(parts, ", ")
}
