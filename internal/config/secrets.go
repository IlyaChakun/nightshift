package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultSecretsDir is where compose mounts secrets (design section 16).
const DefaultSecretsDir = "/run/secrets"

var secretName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

// Secrets reads compose secrets from files. Tokens never come from environment
// variables, which leak into child processes, crash dumps and `docker inspect`.
type Secrets struct {
	Dir string
}

// Read returns the secret's value without the trailing newline.
func (s Secrets) Read(name string) (string, error) {
	if !secretName.MatchString(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("secret %q: invalid name", name)
	}
	dir := s.Dir
	if dir == "" {
		dir = DefaultSecretsDir
	}
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("secret %s: %w", name, err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("secret %s: mode %v is readable by others, want 0400 or 0600", name, info.Mode().Perm())
	}
	raw, err := os.ReadFile(path) //nolint:gosec // name is validated above
	if err != nil {
		return "", fmt.Errorf("secret %s: %w", name, err)
	}
	value := strings.TrimRight(string(raw), "\r\n")
	if value == "" {
		return "", fmt.Errorf("secret %s: %w", name, ErrEmptySecret)
	}
	return value, nil
}

// ErrEmptySecret means the secret file exists but holds nothing.
var ErrEmptySecret = errors.New("empty secret")
