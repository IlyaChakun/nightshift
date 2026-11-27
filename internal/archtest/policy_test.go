package archtest_test

import "strings"

// The one canonical policy of ADR 0026: which plane each package belongs to, which plane may
// import which, and the exact exceptions that exist today. docs/research/architecture-inventory.md
// explains each choice; items of docs/plan/architecture/ remove the exceptions.

type plane string

const (
	root          plane = "composition root"
	exp           plane = "experience"
	control       plane = "control"
	execution     plane = "execution"
	knowledge     plane = "knowledge"
	assurance     plane = "assurance"
	evolution     plane = "evolution"
	observability plane = "observability"
	data          plane = "data"
	custom        plane = "customization"
	testPlane     plane = "testing"
	contracts     plane = "contracts"
)

// planes maps a package path (relative to the module) to its plane. A path matches itself and,
// when it ends in "/...", every package below it; the longest match wins.
var planes = map[string]plane{
	"cmd/nightshift":                        root,
	"internal/adapters":                     root,
	"internal/mcpdoor":                      exp,
	"internal/adapters/chat/slackapp":       exp,
	"internal/adapters/chat/inbox":          exp,
	"internal/observability/summary":        observability,
	"internal/flow":                         control,
	"internal/dispatch":                     control,
	"internal/train":                        control,
	"internal/settings":                     control,
	"internal/deploy":                       control,
	"internal/jobserver":                    control,
	"internal/adapters/github":              control,
	"internal/execution/runner":             execution,
	"internal/execution/jobsapi":            execution,
	"internal/execution/gitops":             execution,
	"internal/trustedgit":                   control,
	"internal/execution/harness/...":        execution,
	"internal/execution/toolchain":          execution,
	"internal/execution/skills":             execution,
	"internal/execution/deployer":           execution,
	"internal/adapters/deploy/composelocal": execution,
	"internal/adapters/deploy/remote":       execution,
	"internal/knowledge/search":             knowledge,
	"internal/knowledge/memory":             knowledge,
	"internal/assurance/check":              assurance,
	"internal/assurance/policy":             assurance,
	"internal/assurance/standards":          assurance,
	"internal/assurance/rubric":             assurance,
	"internal/assurance/deps":               assurance,
	"internal/evolution/learn":              evolution,
	"internal/data/book/...":                data,
	"internal/customization/config":         custom,
	"internal/customization/onboard":        custom,
	"internal/customization/stack":          custom,
	"schemas":                               custom,
	"internal/evals":                        testPlane,
	"internal/testutil/...":                 testPlane,
	"internal/execution/harness/fake":       testPlane,
	"internal/adapters/chat/fake":           testPlane,
	"internal/adapters/deploy/fake":         testPlane,
	"internal/adapters/github/fake":         testPlane,
	"internal/adapters/tracker/fake":        testPlane,
	"deploy":                                testPlane,
	"internal/archtest":                     testPlane,
	"internal/integration":                  contracts,
}

// folders names the folder under internal/ that holds each plane's packages (ADR 0026). A package
// in a plane folder must be classified in that plane; A5 moves the rest in, one plane at a time.
var folders = map[plane]string{
	exp: "experience", control: "control", execution: "execution", knowledge: "knowledge",
	assurance: "assurance", evolution: "evolution", observability: "observability", data: "data",
	custom: "customization", testPlane: "testing",
}

// allowed lists, for each plane, the planes its production code may import. A composition root
// and test code may import anything. A target listed in ports narrows an allowed plane to those
// packages only.
var allowed = map[plane][]plane{
	exp:           {exp, control, observability, data, custom, contracts},
	control:       {control, execution, knowledge, assurance, observability, data, custom, contracts},
	observability: {observability, control, data, custom, contracts},
	execution:     {execution, assurance, custom, contracts},
	knowledge:     {knowledge, data, custom},
	assurance:     {assurance, execution, custom},
	evolution:     {evolution, execution, knowledge, assurance, data, custom},
	data:          {data},
	custom:        {custom},
	contracts:     {contracts},
}

// ports narrows an allowed "from → to" plane pair to the packages under internal/ that are its
// contracts: the Harness interface, the job protocol and the worker's half of Git (gitops, which
// trustedgit builds on) in execution, and the task states of flow, which observability reads and
// never changes.
var ports = map[[2]plane][]string{
	{control, execution}:     {"execution/harness", "execution/jobsapi", "execution/gitops"},
	{assurance, execution}:   {"execution/harness"},
	{evolution, execution}:   {"execution/harness"},
	{observability, control}: {"flow"},
}

// concreteAdapter reports a package that only a composition root or a test may import.
func concreteAdapter(path string) bool {
	return strings.HasPrefix(path, "internal/adapters/") && !strings.HasSuffix(path, "/fake")
}

// workerRoots are the packages under internal/ that `nightshift work` (cmd/nightshift/work.go) wires
// into the worker. No package reachable from them may import the book or a Postgres client.
var workerRoots = []string{
	"execution/runner", "execution/jobsapi", "execution/gitops", "execution/toolchain", "customization/stack", "customization/config",
	"execution/harness", "execution/harness/claude", "execution/harness/codex", "execution/harness/supervise",
}

// bookOnly are package path prefixes the worker side must never import.
var bookOnly = []string{"internal/data/book", "github.com/jackc/pgx"}

// treesAllowed are the only "/..." entries planes may hold: a broad tree would classify new
// packages silently and defeat the unknown-package rule.
var treesAllowed = []string{"internal/execution/harness/...", "internal/data/book/...", "internal/testutil/..."}

// exceptions are today's violations, each removed by the named plan item. A2 fails on any
// violation not listed here and on an exception that no longer matches a violation.
var exceptions = []exception{
	{From: "internal/dispatch", To: "internal/adapters/chat/inbox", Item: "A6", Why: "E3: the inbox door is wired in dispatch"},
	{From: "internal/execution/deployer", To: "internal/adapters/deploy/composelocal", Item: "A6", Why: "E5: deployer may become a composition root"},
	{From: "internal/execution/deployer", To: "internal/adapters/deploy/remote", Item: "A6", Why: "E5: deployer may become a composition root"},
}

// vendorOwners are the only package trees that may import a vendor SDK. The SDKs and the owners
// come from the depguard rule "core" in .golangci.yml (read by depguardCore), so the fast lint
// and this test share one list and the core names no service; TestDepguardOwners keeps this
// list equal to the rule's file exclusions.
var vendorOwners = []string{"internal/execution/harness", "internal/adapters"}
