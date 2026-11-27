package trustedgit

import (
	"slices"
	"testing"
)

// GitHub refuses a bearer header on git over HTTPS.
func TestAuthIsBasicAndNotInTheArguments(t *testing.T) {
	t.Parallel()
	env := Auth{Token: "test-only"}.env()
	want := "GIT_CONFIG_VALUE_0=Authorization: Basic eC1hY2Nlc3MtdG9rZW46dGVzdC1vbmx5"
	if !slices.Contains(env, want) {
		t.Errorf("env: %v", env)
	}
	if (Auth{}).env() != nil {
		t.Error("no token, no header")
	}
}
