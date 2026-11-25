package onboard_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/onboard"
)

func TestSetupReport(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	ok := onboard.RunChecks(t.Context(), &out, []onboard.Check{
		{Name: "config", Run: func(context.Context) (string, error) { return "1 tenant", nil }},
		{Name: "book", Fix: "start postgres", Run: func(context.Context) (string, error) { return "", errors.New("no\nanswer") }},
		{Name: "later", Run: func(context.Context) (string, error) { return "still runs", nil }},
	})
	want := "ok    config: 1 tenant\nfail  book: no answer\n      fix: start postgres\nok    later: still runs\n3 checks, 1 failed\n"
	if ok || out.String() != want {
		t.Errorf("ok=%v\n%s", ok, out.String())
	}
}
