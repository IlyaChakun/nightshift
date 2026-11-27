package archtest_test

import "strings"

// The one canonical policy of ADR 0026: which plane each package belongs to, which plane may
// import which, and the exact exceptions that exist today. docs/research/architecture-inventory.md
// explains each choice; items of docs/plan/architecture/ remove the exceptions.

type plane string

const (
	root      plane = "composition root"
	exp       plane = "experience"
	control   plane = "control"
	execution plane = "execution"
	knowledge plane = "knowledge"
	assurance plane = "assurance"
	evolution plane = "evolution"
	data      plane = "data"
	custom    plane = "customization"
	testPlane plane = "testing"
	contracts plane = "contracts"
)

// planes maps a package path (relative to the module) to its plane. A path matches itself and,
// when it ends in "/...", every package below it; the longest match wins.
var planes = map[string]plane{
	"cmd/nightshift":                        root,
	"internal/adapters":                     root,
	"internal/mcpdoor":                      exp,
	"internal/adapters/chat/slackapp":       exp,
	"internal/adapters/chat/inbox":          exp,
	"internal/summary":                      exp,
	"internal/flow":                         control,
	"internal/dispatch":                     control,
	"internal/train":                        control,
	"internal/settings":                     control,
	"internal/deploy":                       control,
	"internal/adapters/github":              control,
	"internal/runner":                       execution,
	"internal/jobsapi":                      execution,
	"internal/gitops":                       execution,
	"internal/harness/...":                  execution,
	"internal/toolchain":                    execution,
	"internal/skills":                       execution,
	"internal/deployer":                     execution,
	"internal/adapters/deploy/composelocal": execution,
	"internal/adapters/deploy/remote":       execution,
	"internal/search":                       knowledge,
	"internal/memory":                       knowledge,
	"internal/check":                        assurance,
	"internal/policy":                       assurance,
	"internal/standards":                    assurance,
	"internal/rubric":                       assurance,
	"internal/deps":                         assurance,
	"internal/learn":                        evolution,
	"internal/book/...":                     data,
	"internal/config":                       custom,
	"internal/onboard":                      custom,
	"internal/stack":                        custom,
	"schemas":                               custom,
	"internal/evals":                        testPlane,
	"internal/testutil/...":                 testPlane,
	"internal/harness/fake":                 testPlane,
	"internal/adapters/chat/fake":           testPlane,
	"internal/adapters/deploy/fake":         testPlane,
	"internal/adapters/github/fake":         testPlane,
	"internal/adapters/tracker/fake":        testPlane,
	"deploy":                                testPlane,
	"internal/archtest":                     testPlane,
	"internal/integration":                  contracts,
}

// allowed lists, for each plane, the planes its production code may import. A composition root
// and test code may import anything. A target listed in ports narrows an allowed plane to those
// packages only.
var allowed = map[plane][]plane{
	exp:       {exp, control, data, custom, contracts},
	control:   {control, execution, knowledge, assurance, data, custom, contracts},
	execution: {execution, assurance, custom, contracts},
	knowledge: {knowledge, data, custom},
	assurance: {assurance, execution, custom},
	evolution: {evolution, execution, knowledge, assurance, data, custom},
	data:      {data},
	custom:    {custom},
	contracts: {contracts},
}

// ports narrows "from plane → execution" to the packages under internal/ that are contracts of
// execution: the Harness interface, the job protocol and the Git operations control drives.
var ports = map[[2]plane][]string{
	{control, execution}:   {"harness", "jobsapi", "gitops"},
	{assurance, execution}: {"harness"},
	{evolution, execution}: {"harness"},
}

// concreteAdapter reports a package that only a composition root or a test may import.
func concreteAdapter(path string) bool {
	return strings.HasPrefix(path, "internal/adapters/") && !strings.HasSuffix(path, "/fake")
}

// workerRoots are the packages under internal/ that `nightshift work` (cmd/nightshift/work.go) wires
// into the worker. No package reachable from them may import the book or a Postgres client.
var workerRoots = []string{
	"runner", "jobsapi", "gitops", "toolchain", "stack", "config",
	"harness", "harness/claude", "harness/codex", "harness/supervise",
}

// bookOnly are package path prefixes the worker side must never import.
var bookOnly = []string{"internal/book", "github.com/jackc/pgx"}

// treesAllowed are the only "/..." entries planes may hold: a broad tree would classify new
// packages silently and defeat the unknown-package rule.
var treesAllowed = []string{"internal/harness/...", "internal/book/...", "internal/testutil/..."}

// exceptions are today's violations, each removed by the named plan item. A2 fails on any
// violation not listed here and on an exception that no longer matches a violation.
var exceptions = []exception{
	{From: "internal/dispatch", To: "internal/runner", Item: "A4", Why: "E1: runner.PatchName"},
	{From: "internal/jobsapi", To: "internal/book/bookdb", Item: "A4", Why: "E2: the server shares the package with the worker's client"},
	{From: "internal/jobsapi", To: "github.com/jackc/pgx/v5", Item: "A4", Why: "E2: the worker reaches pgx through jobsapi"},
	{From: "internal/jobsapi", To: "github.com/jackc/pgx/v5/pgtype", Item: "A4", Why: "E2: the worker reaches pgx through jobsapi"},
	{From: "internal/jobsapi", To: "github.com/jackc/pgx/v5/pgxpool", Item: "A4", Why: "E2: the worker reaches pgx through jobsapi"},
	{From: "internal/dispatch", To: "internal/adapters/chat/inbox", Item: "A6", Why: "E3: the inbox door is wired in dispatch"},
	{From: "internal/deployer", To: "internal/adapters/deploy/composelocal", Item: "A6", Why: "E5: deployer may become a composition root"},
	{From: "internal/deployer", To: "internal/adapters/deploy/remote", Item: "A6", Why: "E5: deployer may become a composition root"},
	{From: "internal/stack", To: "internal/check", Item: "A5", Why: "E6: stack returns check.Tool"},
	{From: "internal/dispatch", To: "internal/summary", Item: "A6", Why: "E7: dispatch sends the summary it builds"},
}

// vendorOwners are the only package trees that may import a vendor SDK. The SDKs and the owners
// come from the depguard rule "core" in .golangci.yml (read by depguardCore), so the fast lint
// and this test share one list and the core names no service; TestDepguardOwners keeps this
// list equal to the rule's file exclusions.
var vendorOwners = []string{"internal/harness", "internal/adapters"}
