package github_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// appTwin answers the two calls of a GitHub App sign-in, checks the signature of the app's
// proof and then accepts only the token it granted.
func appTwin(t *testing.T, key *rsa.PrivateKey, expires time.Time) (url string, granted *atomic.Int32) {
	t.Helper()
	granted = &atomic.Int32{}
	signedByApp := func(r *http.Request) bool {
		parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
		if len(parts) != 3 {
			return false
		}
		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		claims, cerr := base64.RawURLEncoding.DecodeString(parts[1])
		return err == nil && cerr == nil && strings.Contains(string(claims), `"iss":"12345"`) &&
			rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig) == nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/example-owner/app/installation", func(w http.ResponseWriter, r *http.Request) {
		if !signedByApp(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":7}`))
	})
	mux.HandleFunc("POST /app/installations/7/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Repositories []string }
		if !signedByApp(r) || json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Repositories) != 1 || in.Repositories[0] != "app" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		granted.Add(1)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "installation-token", "expires_at": expires})
	})
	mux.HandleFunc("POST /repos/example-owner/app/issues/1/labels", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer installation-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL, granted
}

func TestAppTokenIsPerRepositoryAndCached(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	now := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	url, granted := appTwin(t, key, now.Add(time.Hour))
	app, err := github.NewApp("12345\n", keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	app.BaseURL, app.Now = url, func() time.Time { return now }
	client := &github.Client{BaseURL: url, TokenFor: app.Token}
	ref := integration.PRRef{Repo: "example-owner/app", Number: 1}
	label := func() {
		t.Helper()
		if err := client.AddLabels(t.Context(), ref, []string{"x"}); err != nil {
			t.Fatal(err)
		}
	}
	label()
	label()
	if n := granted.Load(); n != 1 {
		t.Errorf("a fresh token must be reused, %d granted", n)
	}
	now = now.Add(55 * time.Minute) // close to expiry
	label()
	if n := granted.Load(); n != 2 {
		t.Errorf("an expiring token must be replaced, %d granted", n)
	}
	if _, err := app.Token(t.Context(), "example-owner/other"); err == nil {
		t.Error("a repository without the app must fail")
	}
}

func TestNewAppRefusesBadInputWithoutEchoingIt(t *testing.T) {
	t.Parallel()
	for _, c := range [][2]string{{"", "x"}, {"12345", "secret-not-pem"}, {"12345", "-----BEGIN RSA PRIVATE KEY-----\nc2VjcmV0\n-----END RSA PRIVATE KEY-----\n"}} {
		if _, err := github.NewApp(c[0], c[1]); err == nil || strings.Contains(err.Error(), "secret") {
			t.Errorf("id %q: %v", c[0], err)
		}
	}
}
