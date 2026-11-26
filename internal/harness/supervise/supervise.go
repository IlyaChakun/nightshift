// Package supervise watches one harness turn from outside and stops it when a rule says the
// run is wasting money: no sign of life, the same tool call again and again, or too many
// tokens or dollars. It works on the events adapters emit, so one set of rules serves every
// harness. A harness that emits no events is limited to the stall rule and the turn's timeout.
package supervise

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ilyachakun/nightshift/internal/harness"
)

// Policy holds the limits of a turn. A zero field switches its rule off, so a tenant can
// raise a limit or remove it.
type Policy struct {
	StallAfter  time.Duration // no event for this long
	RepeatLimit int           // the same tool call this many times in a row
	Tokens      int           // input plus output tokens of the turn; cache reads and writes are not counted
	CostUSD     float64       // money spent in the turn
	// Price turns tokens into dollars for a harness whose events carry no cost (the CLIs
	// report it only at the end). Nil means the cost rule sees only reported costs.
	Price func(inputTokens, outputTokens int) float64
}

// Rule looks at each event and returns a reason to stop the run, or "".
type Rule interface {
	Check(ev harness.Event) string
}

// Rules returns the rules the policy asks for. Stall is not a Rule: it needs a clock.
func (p Policy) Rules() []Rule {
	var rs []Rule
	if p.RepeatLimit > 0 {
		rs = append(rs, &Repeat{Limit: p.RepeatLimit})
	}
	if p.Tokens > 0 || p.CostUSD > 0 {
		rs = append(rs, &Spend{Tokens: p.Tokens, CostUSD: p.CostUSD, Price: p.Price})
	}
	return rs
}

// Repeat stops a run that makes the same tool call Limit times in a row.
type Repeat struct {
	Limit int
	last  string
	n     int
}

// Check implements Rule.
func (r *Repeat) Check(ev harness.Event) string {
	if ev.Kind != harness.EventToolCall {
		return ""
	}
	key := ev.Tool + "\x00" + ev.Input
	if key == r.last {
		r.n++
	} else {
		r.last, r.n = key, 1
	}
	if r.n >= r.Limit {
		return fmt.Sprintf("loop: %s called %d times in a row with the same input", ev.Tool, r.n)
	}
	return ""
}

// Spend stops a run that used more than its tokens or dollars.
type Spend struct {
	Tokens  int
	CostUSD float64
	Price   func(inputTokens, outputTokens int) float64 // prices an event that carries no cost
	used    harness.Usage
}

// Check implements Rule.
func (s *Spend) Check(ev harness.Event) string {
	if ev.Kind != harness.EventUsage {
		return ""
	}
	s.used.InputTokens += ev.Usage.InputTokens
	s.used.OutputTokens += ev.Usage.OutputTokens
	cost := ev.Usage.CostUSD
	if cost == 0 && s.Price != nil {
		cost = s.Price(ev.Usage.InputTokens, ev.Usage.OutputTokens)
	}
	s.used.CostUSD += cost
	return s.over(s.used)
}

// over says which limit u passes, or "".
func (s *Spend) over(u harness.Usage) string {
	if total := u.InputTokens + u.OutputTokens; s.Tokens > 0 && total > s.Tokens {
		return fmt.Sprintf("tokens: %d used, limit %d", total, s.Tokens)
	}
	if s.CostUSD > 0 && u.CostUSD > s.CostUSD {
		return fmt.Sprintf("cost: $%.2f spent, limit $%.2f", u.CostUSD, s.CostUSD)
	}
	return ""
}

// Supervisor wraps a harness and enforces a policy on every turn.
type Supervisor struct {
	Next   harness.Harness
	Policy Policy
	// Extra rules run after the ones the policy asks for. A pack or a tenant adds its own here.
	Extra []func() Rule
}

// Run implements harness.Harness. A run the supervisor stopped returns ExitBudget with the
// reason in Output, the same exit a spent budget gets.
func (s Supervisor) Run(ctx context.Context, req harness.RunRequest) (harness.RunResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu     sync.Mutex
		why    string
		over   bool // the harness returned; a late stop must not rewrite its result
		used   harness.Usage
		rules  = s.Policy.Rules()
		alive  = make(chan struct{}, 1)
		finish = make(chan struct{})
	)
	for _, mk := range s.Extra {
		rules = append(rules, mk())
	}
	stop := func(reason string) {
		mu.Lock()
		defer mu.Unlock()
		if why == "" && !over {
			why = reason
			cancel()
		}
	}
	prev := req.Observe
	req.Observe = func(ev harness.Event) {
		if prev != nil {
			prev(ev)
		}
		select {
		case alive <- struct{}{}:
		default:
		}
		mu.Lock()
		if ev.Kind == harness.EventUsage {
			used.InputTokens += ev.Usage.InputTokens
			used.OutputTokens += ev.Usage.OutputTokens
			used.CostUSD += ev.Usage.CostUSD
		}
		mu.Unlock()
		for _, r := range rules {
			if reason := r.Check(ev); reason != "" {
				stop(reason)
				return
			}
		}
	}
	if s.Policy.StallAfter > 0 {
		go watchStall(s.Policy.StallAfter, alive, finish, stop)
	}
	res, err := s.Next.Run(ctx, req)
	close(finish)

	mu.Lock()
	defer mu.Unlock()
	over = true
	if why == "" {
		if why = s.overAtEnd(res, err); why == "" {
			return res, err
		}
	}
	out, _ := json.Marshal(map[string]string{"stopped": why})
	// A CLI killed before its result line reports less than the supervisor saw: keep the larger.
	res.Usage.InputTokens = max(res.Usage.InputTokens, used.InputTokens)
	res.Usage.OutputTokens = max(res.Usage.OutputTokens, used.OutputTokens)
	res.Usage.CostUSD = max(res.Usage.CostUSD, used.CostUSD)
	return harness.RunResult{Output: out, SessionID: res.SessionID, Usage: res.Usage, ExitReason: harness.ExitBudget}, nil
}

// overAtEnd is the backstop: an adapter's events can undercount (the claude CLI's assistant
// lines carry a usage snapshot), so a finished run is checked again against its final token
// count. Cost is not checked here: on the subscription the CLI's cost is notional, and a run
// that already did its work should not be thrown away for it.
func (s Supervisor) overAtEnd(res harness.RunResult, err error) string {
	if err != nil || res.ExitReason != harness.ExitDone {
		return ""
	}
	if why := (&Spend{Tokens: s.Policy.Tokens}).over(res.Usage); why != "" {
		return why + " (final count)"
	}
	return ""
}

// watchStall calls stop when no sign of life arrives for d, and returns when the run ends.
func watchStall(d time.Duration, alive, finish <-chan struct{}, stop func(string)) {
	t := time.NewTimer(d)
	defer t.Stop()
	for {
		select {
		case <-alive:
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			t.Reset(d)
		case <-t.C:
			stop(fmt.Sprintf("stall: no output for %s", d))
			return
		case <-finish:
			return
		}
	}
}
