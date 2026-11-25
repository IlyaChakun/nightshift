package skills

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// file is one regular file of a skill as it sits in the repository.
type file struct {
	Name string
	Exec bool
	Link bool // symlink or special file: it can point outside the skill
	Data []byte
}

var scriptExt = map[string]bool{
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".py": true, ".js": true, ".mjs": true,
	".cjs": true, ".ts": true, ".rb": true, ".pl": true, ".php": true, ".ps1": true, ".bat": true,
	".cmd": true, ".exe": true, ".dll": true, ".so": true, ".dylib": true, ".jar": true, ".wasm": true,
}

// scan returns one finding per obvious problem: a skill is text for the model, so scripts,
// binaries and characters a reader cannot see are refused (design section 12, "Security").
func scan(files []file) []string {
	var out []string
	for _, f := range files {
		add := func(why string) { out = append(out, f.Name+": "+why) }
		switch {
		case f.Link:
			add("link or special file")
		case scriptExt[strings.ToLower(path.Ext(f.Name))]:
			add("script or binary file by extension")
		case f.Exec:
			add("executable bit")
		case bytes.HasPrefix(f.Data, []byte("#!")):
			add("starts with a shebang")
		case bytes.IndexByte(f.Data, 0) >= 0 || !utf8.Valid(f.Data):
			add("binary or not UTF-8")
		default:
			line := 1
			for _, r := range string(f.Data) {
				if r == '\n' {
					line++
				} else if unicode.Is(unicode.Cf, r) {
					add(fmt.Sprintf("hidden unicode U+%04X at line %d", r, line))
					break
				}
			}
		}
	}
	return out
}

// leaks runs gitleaks over dir when it is installed. The bool says whether it ran.
func leaks(ctx context.Context, dir string) (string, bool) {
	bin, err := exec.LookPath("gitleaks")
	if err != nil {
		return "", false
	}
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "dir", "--no-banner", "--redact", dir)
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return "gitleaks: " + strings.TrimSpace(buf.String()), true
	}
	return "", true
}
