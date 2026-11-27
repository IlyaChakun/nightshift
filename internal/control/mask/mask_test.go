package mask

import (
	"encoding/json"
	"strings"
	"testing"
)

// fake builds token-shaped test strings at run time, so secret scanners do not flag the source.
func fake(prefix string, n int) string { return prefix + strings.Repeat("a1", n/2) }

func TestMaskString(t *testing.T) {
	t.Parallel()
	m := NewMasker("instance-secret-value", "short")
	tests := map[string]string{ //nolint:gosec // synthetic token shapes built at run time
		"token " + fake("xoxb-", 24) + " in text":   "token [masked] in text",
		"key " + fake("sk-ant-api03-", 40):          "key [masked]",
		"pat " + fake("ghp_", 36):                   "pat [masked]",
		"cloud " + "AKIA" + strings.Repeat("Q7", 8): "cloud [masked]",
		"Authorization: Bearer " + fake("", 32):     "Authorization: [masked]",
		"leaked instance-secret-value here":         "leaked [masked] here",
		"short words stay":                          "short words stay",
		"-----BEGIN " + "OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----": "[masked]",
	}
	for in, want := range tests {
		if got := m.String(in); got != want {
			t.Errorf("String(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskURLPassword(t *testing.T) {
	t.Parallel()
	pw := fake("", 48)
	m := NewMasker("postgres://nightshift:" + pw + "@postgres:5432/book?sslmode=disable")
	tests := map[string]string{
		"dial postgres://nightshift:" + pw + "@postgres:5432/book?sslmode=disable": "dial [masked]",
		"password " + pw + " rejected":                                             "password [masked] rejected",
		"user nightshift on postgres:5432":                                         "user nightshift on postgres:5432",
	}
	for in, want := range tests {
		if got := m.String(in); got != want {
			t.Errorf("String(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskJSON(t *testing.T) {
	t.Parallel()
	m := NewMasker("instance-secret-value")
	got, err := m.JSON([]byte(`{"text":"see instance-secret-value","blocks":[{"t":"` + fake("xoxb-", 24) + `"}],"n":3}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"blocks":[{"t":"[masked]"}],"n":3,"text":"see [masked]"}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := m.JSON([]byte(`not json`)); err == nil {
		t.Error("invalid JSON accepted")
	}
}

// FuzzMaskJSON: masking never breaks JSON and never lets a known secret through.
func FuzzMaskJSON(f *testing.F) {
	const secret = "instance-secret-value"
	f.Add(`{"a":"instance-secret-value"}`)
	f.Add(`["` + fake("xoxb-", 24) + `", 1, null]`)
	f.Add(`{"instance-secret-value": {"x": "y"}}`)
	m := NewMasker(secret)
	f.Fuzz(func(t *testing.T, doc string) {
		if !json.Valid([]byte(doc)) {
			return
		}
		out, err := m.JSON([]byte(doc))
		if err != nil {
			t.Fatalf("valid JSON rejected: %v", err)
		}
		if !json.Valid(out) {
			t.Fatalf("output is not JSON: %s", out)
		}
		var v any
		_ = json.Unmarshal(out, &v)
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), secret) {
			t.Fatalf("secret survived: %s", out)
		}
	})
}
