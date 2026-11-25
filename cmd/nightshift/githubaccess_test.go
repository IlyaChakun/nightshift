package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
)

func TestGitHubAccess(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		files   map[string]string
		on      bool
		wantErr bool
	}{
		"nothing":            {},
		"empty files":        {files: map[string]string{"github_token": "", "github_app_id": "", "github_app_key": ""}},
		"token":              {files: map[string]string{"github_token": "test-only"}, on: true},
		"half an app":        {files: map[string]string{"github_app_id": "12345", "github_token": "test-only"}, wantErr: true},
		"app with a bad key": {files: map[string]string{"github_app_id": "12345", "github_app_key": "not pem"}, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for file, value := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, file), []byte(value), 0o400); err != nil {
					t.Fatal(err)
				}
			}
			got, err := githubAccess(config.Secrets{Dir: dir})
			if (err != nil) != tc.wantErr || (got != nil) != tc.on {
				t.Errorf("access %v, error %v", got, err)
			}
		})
	}
}
