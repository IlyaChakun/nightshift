package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/customization/config"
)

// answers are what only the person knows about a new instance.
type answers struct {
	tenant, repo, owner, member, channel, providers, review, claude, codex string
}

// knownProviders are the providers `setup` can write (design section 12).
var knownProviders = map[string]map[string]any{
	"claude-sub": {"harness": "claude", "auth": "subscription", "concurrency": 1, "batch": "forbidden"},
	"claude-api": {"harness": "claude", "auth": "api_key", "key_env": "ANTHROPIC_API_KEY"},
	"codex-api":  {"harness": "codex", "auth": "api_key", "key_env": "OPENAI_API_KEY"},
}

var versionRe = regexp.MustCompile(`^$|^[0-9]+(\.[0-9]+)+[A-Za-z0-9.-]*$`)

// setupInstance handles `nightshift setup`: it asks for what it cannot know and writes the
// config, providers, tenant and deploy/.env. It writes no secret and overwrites nothing
// unless asked.
func setupInstance(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var a answers
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "deploy", "the deploy directory to write into")
	replace := fs.Bool("replace", false, "overwrite files that exist")
	questions := []struct {
		value          *string
		name, def, ask string
	}{
		{&a.tenant, "tenant", "example-solo", "Tenant name"},
		{&a.repo, "repo", "", "Repository the factory works on (owner/name)"},
		{&a.owner, "owner", "", "Your GitHub login (empty: the repository's owner)"},
		{&a.member, "member", "", "Your chat member id (empty: no chat app, you answer from the terminal or your own Claude)"},
		{&a.channel, "channel", "", "Channel id for tasks (empty: no chat app)"},
		{&a.providers, "providers", "claude-sub", "Providers for Builder, in order (claude-sub, claude-api, codex-api)"},
		{&a.review, "review", "all", "Review mode (all, blast_radius, off)"},
		{&a.claude, "claude-version", "", "Claude Code version for the worker (empty: leave out)"},
		{&a.codex, "codex-version", "", "Codex version for the worker (empty: leave out)"},
	}
	for _, q := range questions {
		fs.StringVar(q.value, q.name, q.def, q.ask)
	}
	ask := fs.Bool("ask", true, "ask for every answer not given as a flag")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr, setupUsage)
		return exitUsage
	}
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	lines := bufio.NewScanner(stdin)
	for _, q := range questions {
		if !*ask || given[q.name] {
			continue
		}
		_, _ = fmt.Fprintf(stdout, "%s [%s]: ", q.ask, q.def)
		if lines.Scan() {
			if text := strings.TrimSpace(lines.Text()); text != "" {
				*q.value = text
			}
		}
	}
	written, err := writeInstance(*out, a, *replace)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift setup:", err)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stdout, "\nwrote %s\nnext: docker compose up -d --build, then `nightshift doctor` in nightshift and `nightshift doctor --worker` in the worker\n", strings.Join(written, ", "))
	if a.member == "" && a.channel == "" {
		_, _ = fmt.Fprintln(stdout, "no chat app: answer Lead with `docker compose exec nightshift nightshift inbox` and `reply`, or connect your own Claude with\n  claude mcp add nightshift -- docker compose exec -T nightshift nightshift mcp")
	}
	return exitOK
}

// files renders the files of an instance, by path under the deploy directory.
func (a answers) files() (map[string][]byte, error) {
	owner, name, ok := strings.Cut(a.repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("the repository must be owner/name, not %q", a.repo)
	}
	if !tenantName.MatchString(a.tenant) {
		return nil, fmt.Errorf("%q is not a tenant name (lowercase letters, digits, dashes)", a.tenant)
	}
	if a.owner == "" {
		a.owner = owner
	}
	if !versionRe.MatchString(a.claude) || !versionRe.MatchString(a.codex) {
		return nil, errors.New("a CLI version looks like 1.2.3")
	}
	providers := map[string]any{}
	var chain []string
	for p := range strings.SplitSeq(a.providers, ",") {
		p = strings.TrimSpace(p)
		def, known := knownProviders[p]
		if !known {
			return nil, fmt.Errorf("unknown provider %q", p)
		}
		providers[p], chain = def, append(chain, p)
	}
	// Without a chat app the tenant uses the inbox: cards live in the book and the terminal or
	// a Claude client answers them (ADR 0006). The doors act as the owner's chat id "local".
	chat := map[string]any{"adapter": "inbox"}
	person := map[string]any{"github": a.owner, "chat": "local"}
	switch {
	case a.member != "" && a.channel != "":
		chat = map[string]any{"adapter": "slack-app", "channels": map[string]string{"planning": a.channel}}
		person["chat"] = a.member
	case a.member != "" || a.channel != "":
		return nil, errors.New("chat needs both the member id and the channel id, or neither")
	}
	docs := map[string]any{
		"config/instance.yaml": map[string]any{
			"name": "personal", "trust_domain": "personal", "providers": "providers.yaml", "tenants": "tenants/",
			"egress": map[string]any{"allow": []string{"api.anthropic.com", "api.openai.com", "proxy.golang.org", "sum.golang.org", "pypi.org", "files.pythonhosted.org", "registry.npmjs.org", "api.deps.dev", "api.osv.dev"}},
		},
		"config/providers.yaml": providers,
		"config/tenants/" + a.tenant + "/tenant.yaml": map[string]any{
			"name": a.tenant, "governance": "solo", "data_classification": "internal",
			"repos":  []string{"github.com/" + a.repo},
			"people": map[string]any{"owner": person, "approvers": []string{"owner"}},
			"chat":   chat, "tracker": map[string]any{"adapter": "linear", "project": "EX"},
			"docs":      map[string]any{"adapter": "git", "path": "docs/"},
			"providers": map[string]any{"builder": chain},
			"review":    map[string]any{"mode": a.review},
			"schedule":  map[string]any{"timezone": "Etc/UTC"},
		},
	}
	files := map[string][]byte{}
	for path, doc := range docs {
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", path, err)
		}
		files[path] = raw
	}
	var env strings.Builder
	env.WriteString("NIGHTSHIFT_OTLP=http://lgtm:4318\nNIGHTSHIFT_TELEMETRY=1\n")
	for _, v := range [][2]string{{"CLAUDE_CODE_VERSION", a.claude}, {"CODEX_VERSION", a.codex}} {
		if v[1] != "" {
			_, _ = fmt.Fprintf(&env, "%s=%s\n", v[0], v[1])
		}
	}
	files[".env"] = []byte(env.String())
	return files, nil
}

// writeInstance validates the rendered files as the factory would load them and only then puts
// them into dir. It returns what it wrote.
func writeInstance(dir string, a answers, replace bool) ([]string, error) {
	files, err := a.files()
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp("", "nightshift-setup-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if err := put(scratch, files); err != nil {
		return nil, err
	}
	if _, err := config.LoadInstance(config.NewValidator(), filepath.Join(scratch, "config")); err != nil {
		return nil, fmt.Errorf("the answers do not make a valid instance: %w", err)
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil && !replace {
			return nil, fmt.Errorf("%s exists; nothing was written (pass --replace to overwrite)", filepath.Join(dir, path))
		}
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths, put(dir, files)
}

// put writes files under dir. They hold no secret, and containers of other users read them.
func put(dir string, files map[string][]byte) error {
	for path, body := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil { //nolint:gosec // the nightshift container reads the config
			return err
		}
		if err := os.WriteFile(full, body, 0o644); err != nil { //nolint:gosec // see above
			return err
		}
	}
	return nil
}
