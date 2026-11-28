package dispatch_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/jobserver"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/golden"
)

const evidenceAttempt = "00112233aabbccdd"

const fakeSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// completeAcceptance finishes the queued acceptance job like a worker that left a screenshot, a
// video, a copy of its scenarios and a log in its artifact directory, after 3m12s for $0.42.
func (r *rig) completeAcceptance(result string) {
	r.t.Helper()
	const workerToken = "worker-fixture"
	api := httptest.NewServer((&jobserver.Server{Pool: r.c.Pool, WorkerTokenSHA256: jobserver.HashToken(workerToken)}).Handler())
	defer api.Close()
	client := &jobsapi.Client{BaseURL: api.URL, WorkerToken: workerToken, HTTP: api.Client()}
	claim, err := client.Claim(r.t.Context(), "fixture", 0)
	if err != nil {
		r.t.Fatal(err)
	}
	// The runner names evidence files; a worker that reports other paths gets them left off.
	// Each file has its own content, except 03.png, a second save of 01.png (V4 on the Mac): the
	// card shows it once.
	var arts []jobsapi.Artifact
	for i, p := range []string{evidenceAttempt + "/evidence/01.png", evidenceAttempt + "/evidence/02.webm", evidenceAttempt + "/scenarios.json",
		evidenceAttempt + "/browser.log", evidenceAttempt + "/evidence/the-export-holds-exactly-10001-rows.png", "1/home.png", evidenceAttempt + "/evidence/03.png"} {
		sha := fmt.Sprintf("%064x", i)
		if strings.HasSuffix(p, "/03.png") {
			sha = fmt.Sprintf("%064x", 0)
		}
		arts = append(arts, jobsapi.Artifact{Path: p, SHA256: sha, Bytes: 10})
	}
	if err := client.Complete(r.t.Context(), claim.JobID, claim.JobToken, jobsapi.CompleteRequest{
		ExitReason: "done", Result: json.RawMessage(result), Artifacts: arts, Usage: jobsapi.Usage{CostUSD: 0.42},
	}); err != nil {
		r.t.Fatal(err)
	}
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.jobs SET started_at = finished_at - interval '3 minutes 12 seconds' WHERE id = $1`, claim.JobID); err != nil {
		r.t.Fatal(err)
	}
}

// evidenceCards are the evidence cards queued for the rig's task.
func (r *rig) evidenceCards() []chatapi.Card { return r.cardsKeyed("evidence:%") }

// cardsKeyed are the cards queued for the rig's task whose outbox key matches like.
func (r *rig) cardsKeyed(like string) []chatapi.Card {
	r.t.Helper()
	rows, err := r.c.Pool.Query(r.t.Context(), `SELECT payload FROM book.outbox WHERE task_id = $1 AND idempotency_key LIKE $2 ORDER BY id`, r.task.ID, like)
	if err != nil {
		r.t.Fatal(err)
	}
	defer rows.Close()
	var cards []chatapi.Card
	for rows.Next() {
		var raw []byte
		var c chatapi.Card
		if err := rows.Scan(&raw); err != nil {
			r.t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			r.t.Fatalf("outbox card %s: %v", raw, err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		r.t.Fatal(err)
	}
	return cards
}

// T4: a passing acceptance puts its evidence on the "Ready for acceptance" card: the preview,
// the screenshots and video, the duration and cost, and never the hidden scenarios (design
// section 9, #deployments; plan P5b).
func TestAPassingAcceptanceCardCarriesItsEvidence(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		t.Run(lang, func(t *testing.T) {
			r := newRig(t)
			r.c.Outbox = &outbox.Outbox{Pool: r.c.Pool, Mask: mask.NewMasker()}
			r.c.ChatLanguage = lang
			r.tenant.People.Owner.Chat = "U1"
			r.withVerifier()
			r.toAcceptance()
			if cards := r.evidenceCards(); len(cards) != 0 {
				t.Fatalf("a card before the verdict: %+v", cards)
			}
			r.completeAcceptance(`{"verdict":"pass","summary":"HS-1 passed"}`)
			r.advance()
			r.advance() // a later tick queues no second card
			cards := r.evidenceCards()
			if r.task.State != "human_review" || len(cards) != 1 {
				t.Fatalf("state %s, %d cards", r.task.State, len(cards))
			}
			card := cards[0]
			if card.Kind != chatapi.CardReady || card.Thread.Channel != "U1" {
				t.Fatalf("card %+v", card)
			}
			if strings.Contains(card.Text, "scenarios.json") || strings.Contains(card.Text, hiddenStep) || strings.Contains(card.Text, "browser.log") || strings.Contains(card.Text, "home.png") {
				t.Fatalf("the card shows more than the evidence:\n%s", card.Text)
			}
			// IDs differ between books; the golden file holds the text with them named.
			text := strings.NewReplacer("#"+strconv.FormatInt(r.task.ID, 10)+" ", "#TASK ", "/"+strconv.FormatInt(r.verifierJob().ID, 10)+"/", "/JOB/").Replace(card.Text)
			golden.Assert(t, filepath.Join("..", "..", "..", "testdata", "golden", "evidence-card-"+lang+".txt"), []byte(text+"\n"))
		})
	}
}

// A failed acceptance queues no evidence card.
func TestAFailedAcceptanceQueuesNoEvidenceCard(t *testing.T) {
	r := newRig(t)
	r.c.Outbox = &outbox.Outbox{Pool: r.c.Pool, Mask: mask.NewMasker()}
	r.tenant.People.Owner.Chat = "U1"
	r.withVerifier()
	r.toAcceptance()
	r.completeAcceptance(`{"verdict":"return","summary":"export is short","findings":[{"rule_id":"new","category":"code","severity":"block","layer_caught":"L5","message":"the exported report misses its last row","fingerprint":"acceptance:last-row"}]}`)
	r.advance()
	if r.task.State != "building" || len(r.evidenceCards()) != 0 {
		t.Fatalf("state %s, cards %+v", r.task.State, r.evidenceCards())
	}
}
