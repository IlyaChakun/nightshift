// Package mask hides secrets in text that leaves the host: the instance's own secret values and
// anything shaped like a token. The outbox masks every payload with it.
package mask

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Masked replaces every secret found in outgoing text.
const Masked = "[masked]"

// tokenShapes are credential formats that must never leave the host, known or not.
var tokenShapes = regexp.MustCompile(strings.Join([]string{
	`xox[abposr]-[A-Za-z0-9-]{10,}`,            // chat bot and user tokens
	`xapp-[A-Za-z0-9-]{10,}`,                   // chat app-level tokens
	`sk-ant-[A-Za-z0-9_-]{20,}`,                // model API keys
	`sk-[A-Za-z0-9]{32,}`,                      // other model API keys
	`(gh[opsu]_|github_pat_)[A-Za-z0-9_]{20,}`, // GitHub tokens
	`glpat-[A-Za-z0-9_-]{20,}`,                 // GitLab tokens
	`AKIA[0-9A-Z]{16}`,                         // cloud access key ids
	`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`,
	`(?i)bearer\s+[A-Za-z0-9._~+/-]{20,}=*`,
}, "|"))

// Masker hides the instance's own secret values and anything shaped like a token.
type Masker struct {
	values []string
}

// NewMasker masks the given secret values (shorter than 8 characters are ignored, they
// would mask ordinary words) plus every known token shape. A value that is a URL with a
// password, such as a database URL, also masks the password alone, because errors and
// logs often quote it without the rest of the URL.
func NewMasker(secrets ...string) *Masker {
	m := &Masker{}
	for _, s := range secrets {
		for _, v := range []string{s, urlPassword(s)} {
			if len(v) >= 8 {
				m.values = append(m.values, v)
			}
		}
	}
	// Longest first, so a secret that contains another is masked whole.
	sort.Slice(m.values, func(i, j int) bool { return len(m.values[i]) > len(m.values[j]) })
	return m
}

// String masks one string.
func (m *Masker) String(s string) string {
	for _, v := range m.values {
		s = strings.ReplaceAll(s, v, Masked)
	}
	return tokenShapes.ReplaceAllString(s, Masked)
}

// JSON masks every string, key or value, inside a JSON document.
func (m *Masker) JSON(doc []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		return nil, fmt.Errorf("mask: decode payload: %w", err)
	}
	out, err := json.Marshal(m.walk(v))
	if err != nil {
		return nil, fmt.Errorf("mask: encode payload: %w", err)
	}
	return out, nil
}

func (m *Masker) walk(v any) any {
	switch t := v.(type) {
	case string:
		return m.String(t)
	case []any:
		for i := range t {
			t[i] = m.walk(t[i])
		}
		return t
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[m.String(k)] = m.walk(val)
		}
		return out
	default:
		return v
	}
}

// urlPassword returns the password inside a URL's user info, or "" if there is none.
func urlPassword(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return ""
	}
	pw, _ := u.User.Password()
	return pw
}
