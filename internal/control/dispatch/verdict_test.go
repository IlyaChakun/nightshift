package dispatch

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/policy"
	"github.com/ilyachakun/nightshift/internal/control/flow"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		cur     policy.Level
		returns int
		event   flow.Event
		raised  policy.Level
		escal   bool
		wantErr bool
	}{
		{"pass", `{"verdict":"pass","summary":"ok"}`, policy.Low, 0, flow.VerifierOK, policy.Low, false, false},
		{"return", `{"verdict":"return","summary":"x"}`, policy.Low, 1, flow.Returned, policy.Low, false, false},
		{"return past the limit escalates", `{"verdict":"return","summary":"x"}`, policy.Low, 3, flow.BudgetExhausted, policy.Low, true, false},
		{"escalate", `{"verdict":"escalate","summary":"x","escalation_reason":"spec is wrong"}`, policy.Low, 0, flow.BudgetExhausted, policy.Low, true, false},
		{"unknown verdict escalates", `{"verdict":"maybe"}`, policy.Low, 0, flow.BudgetExhausted, policy.Low, true, false},
		{"raises the radius", `{"verdict":"pass","summary":"x","raise_blast_radius":{"level":"critical","reason":"auth"}}`, policy.Medium, 0, flow.VerifierOK, policy.Critical, false, false},
		{"never lowers the radius", `{"verdict":"pass","summary":"x","raise_blast_radius":{"level":"high","reason":"r"}}`, policy.Critical, 0, flow.VerifierOK, policy.Critical, false, false},
		{"not JSON", `nope`, policy.Low, 0, "", policy.Low, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decide([]byte(tt.raw), tt.cur, tt.returns, 3)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Event != tt.event || got.Raised != tt.raised || (got.Why != "") != tt.escal {
				t.Errorf("got %+v, want event %s raised %s escalate %v", got, tt.event, tt.raised, tt.escal)
			}
		})
	}
}
