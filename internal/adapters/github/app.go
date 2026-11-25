package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// appIDRe accepts the numeric App ID and the newer client id.
var appIDRe = regexp.MustCompile(`^[A-Za-z0-9.]{1,64}$`)

// refreshBefore is how long before it expires a cached installation token is replaced, so a
// push that starts with it also ends with it.
const refreshBefore = 10 * time.Minute

// App signs in as a GitHub App (design section 16): it trades the app's private key for a
// short-lived token of one repository. Pull requests are then the app's, so a person can
// review them. It is safe for concurrent use.
type App struct {
	BaseURL string // defaults to DefaultBaseURL
	HTTP    *http.Client
	Now     func() time.Time

	id  string
	key *rsa.PrivateKey

	mu     sync.Mutex
	tokens map[string]appToken
}

type appToken struct {
	value   string
	expires time.Time
}

// NewApp reads the app's id and its PEM private key. Neither appears in an error.
func NewApp(id, keyPEM string) (*App, error) {
	id = strings.TrimSpace(id)
	if !appIDRe.MatchString(id) {
		return nil, errors.New("github app: the app id must be the number or client id from the app's page")
	}
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("github app: the private key is not PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return &App{id: id, key: key}, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("github app: the private key cannot be read")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("github app: the private key is not an RSA key")
	}
	return &App{id: id, key: key}, nil
}

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Token returns a token limited to repo (owner/name), from the cache while it is fresh.
func (a *App) Token(ctx context.Context, repo string) (string, error) {
	base, err := repoPath(repo)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if t, ok := a.tokens[repo]; ok && a.now().Add(refreshBefore).Before(t.expires) {
		return t.value, nil
	}
	jwt, err := a.jwt()
	if err != nil {
		return "", err
	}
	api := &Client{BaseURL: a.BaseURL, HTTP: a.HTTP, Token: jwt}
	var installation struct {
		ID int64 `json:"id"`
	}
	if err := api.do(ctx, http.MethodGet, base+"/installation", nil, &installation); err != nil {
		return "", fmt.Errorf("github app: is it installed on %s? %w", repo, err)
	}
	var granted struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	_, name, _ := strings.Cut(repo, "/")
	path := "/app/installations/" + strconv.FormatInt(installation.ID, 10) + "/access_tokens"
	if err := api.do(ctx, http.MethodPost, path, map[string]any{"repositories": []string{name}}, &granted); err != nil {
		return "", fmt.Errorf("github app: token for %s: %w", repo, err)
	}
	if a.tokens == nil {
		a.tokens = map[string]appToken{}
	}
	a.tokens[repo] = appToken{value: granted.Token, expires: granted.ExpiresAt}
	return granted.Token, nil
}

// jwt is the app's own short proof of identity: RS256 over the id, valid for nine minutes
// and dated a minute back for clocks that differ.
func (a *App) jwt() (string, error) {
	now := a.now()
	claims, err := json.Marshal(map[string]any{
		"iss": a.id, "iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("github app: %w", err)
	}
	enc := base64.RawURLEncoding
	signed := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("github app: sign: %w", err)
	}
	return signed + "." + enc.EncodeToString(sig), nil
}
