package harness

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/ilyachakun/nightshift/schemas"
)

type stub struct{ res RunResult }

func (s stub) Run(context.Context, RunRequest) (RunResult, error) { return s.res, nil }

func TestValidating(t *testing.T) {
	t.Parallel()
	verdict, err := fs.ReadFile(schemas.FS, "verdict.json")
	if err != nil {
		t.Fatal(err)
	}
	inline := []byte(`{"type":"object","required":["ok"]}`)
	tests := []struct {
		name    string
		schema  []byte
		res     RunResult
		wantErr error
	}{
		{"embedded schema, valid", verdict, RunResult{ExitReason: ExitDone, Output: []byte(`{"verdict":"pass","summary":"ok"}`)}, nil},
		{"embedded schema with nested ref", verdict, RunResult{ExitReason: ExitDone, Output: []byte(`{"verdict":"return","summary":"s","findings":[{"rule_id":"x"}]}`)}, ErrInvalidOutput},
		{"inline schema, valid", inline, RunResult{ExitReason: ExitDone, Output: []byte(`{"ok":true}`)}, nil},
		{"inline schema, invalid", inline, RunResult{ExitReason: ExitDone, Output: []byte(`{}`)}, ErrInvalidOutput},
		{"output is not json", inline, RunResult{ExitReason: ExitDone, Output: []byte(`nope`)}, ErrInvalidOutput},
		{"budget exit is not validated", inline, RunResult{ExitReason: ExitBudget}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := Validating{Next: stub{tt.res}}
			_, err := h.Run(context.Background(), RunRequest{OutputSchema: tt.schema})
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
