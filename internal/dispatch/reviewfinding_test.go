package dispatch

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ilyachakun/nightshift/internal/integration"
)

func TestReviewFindingCarriesTheReviewersWords(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, body, want string }{
		{"text", " Add a test. \n", "owner requested changes on the pull request:\nAdd a test."},
		{"empty", "  ", "owner requested changes on the pull request and wrote no comment."},
	} {
		if got := reviewFinding(integration.Review{User: "owner", Body: tc.body}); got != tc.want {
			t.Errorf("%s: %q", tc.name, got)
		}
	}
	token := "ghp_" + strings.Repeat("a", 36)
	if got := reviewFinding(integration.Review{User: "owner", Body: "log: " + token}); strings.Contains(got, token) {
		t.Errorf("a token reached the finding: %q", got)
	}
	long := reviewFinding(integration.Review{User: "owner", Body: strings.Repeat("я", maxReviewText+10)})
	if n := utf8.RuneCountInString(long); n > maxReviewText+100 || !strings.HasSuffix(long, "(cut)") {
		t.Errorf("a long review is not cut: %d runes", n)
	}
}
