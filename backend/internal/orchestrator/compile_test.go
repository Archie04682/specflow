package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPlanJSONRendersDurationAndOmitsEmptyModel(t *testing.T) {
	plan := &WorkflowPlan{
		ID:    "plan",
		Nodes: []PlanNode{{ID: "a", Kind: NodeKindAgent, Harness: "fake", Timeout: Duration(20 * time.Minute)}},
		Order: []string{"a"},
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"timeout":"20m"`) {
		t.Fatalf("timeout was not rendered as a duration string: %s", data)
	}
	if strings.Contains(string(data), `"model"`) {
		t.Fatalf("an empty model was carried into the JSON: %s", data)
	}

	var decoded WorkflowPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Nodes[0].Timeout.Duration() != 20*time.Minute {
		t.Fatalf("timeout round-trip = %s", decoded.Nodes[0].Timeout)
	}
}

func TestDurationRendersCompact(t *testing.T) {
	tests := []struct {
		value time.Duration
		want  string
	}{
		{value: 20 * time.Minute, want: "20m"},
		{value: time.Hour, want: "1h"},
		{value: time.Hour + 30*time.Minute, want: "1h30m"},
		{value: time.Minute + 30*time.Second, want: "1m30s"},
		{value: time.Second + 500*time.Millisecond, want: "1.5s"},
		{value: time.Hour + 500*time.Millisecond, want: "1h0.5s"},
		{value: -time.Hour - 30*time.Minute, want: "-1h30m"},
		{value: 500 * time.Millisecond, want: "500ms"},
		{value: 0, want: "0s"},
	}

	for _, test := range tests {
		if got := Duration(test.value).String(); got != test.want {
			t.Fatalf("%s rendered as %q, want %q", test.value, got, test.want)
		}
	}
}

func TestDocumentedAgentTimeoutCompiles(t *testing.T) {
	workflow, err := ParseWorkflow([]byte(`
id: documented-timeout
defaults: {harness: fake}
nodes:
  - {id: apply, kind: agent, prompt: apply the change, timeout: 45m}
`), "documented.yaml")
	if err != nil {
		t.Fatalf("parse documented workflow: %v", err)
	}

	plan, err := NormalizeWorkflow(workflow, NormalizeContext{})
	if err != nil {
		t.Fatalf("normalize documented workflow: %v", err)
	}
	if got := plan.Node("apply").Timeout.Duration(); got != 45*time.Minute {
		t.Fatalf("timeout = %s, want 45m", got)
	}
}

func TestParseWorkflowUsesSourceAsFallbackID(t *testing.T) {
	source := filepath.Join("tmp", "workflow.yaml")
	workflow, err := ParseWorkflow([]byte("nodes:\n  - {id: a, kind: command, command: \"true\"}\n"), source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if workflow.ID != source {
		t.Fatalf("id = %q, want %q", workflow.ID, source)
	}
}

func TestParseWorkflowRejectsMalformedYAML(t *testing.T) {
	if _, err := ParseWorkflow([]byte("nodes: [\n"), "broken.yaml"); err == nil {
		t.Fatal("expected malformed YAML to be rejected")
	}
}

func TestNormalizeWorkflowResolvesValues(t *testing.T) {
	workflow := &Workflow{
		ID: "normalize",
		Defaults: Defaults{
			Harness:     "pi",
			Model:       ModelValue{ID: "cheap", Thinking: "low"},
			Interactive: boolPointer(true),
			AllowInject: boolPointer(true),
		},
		Nodes: []Node{
			{ID: "a", Kind: NodeKindAgent},
			{
				ID:          "b",
				Kind:        NodeKindAgent,
				Harness:     "other",
				Model:       ModelValue{ID: "default"},
				Interactive: boolPointer(false),
			},
			{ID: "c", Kind: NodeKindCommand, Command: "true", Timeout: "5m"},
			{ID: "d", Kind: NodeKindAgent, Model: ModelValue{Thinking: "high"}},
		},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "d"}},
	}

	plan, err := NormalizeWorkflow(workflow, NormalizeContext{})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	a := plan.Node("a")
	if a.Harness != "pi" || a.Model == nil || a.Model.ID != "cheap" || a.Model.Thinking != "low" {
		t.Fatalf("node a = %+v", a)
	}
	if !a.Interactive || !a.AllowInject || a.Workdir != WorkdirRun {
		t.Fatalf("node a permissions/workdir = %+v", a)
	}
	if a.Timeout.Duration() != 30*time.Minute {
		t.Fatalf("node a timeout = %s", a.Timeout)
	}

	b := plan.Node("b")
	if b.Harness != "other" || b.Interactive {
		t.Fatalf("node b = %+v", b)
	}
	if b.Model == nil || b.Model.ID != "" || b.Model.Thinking != "low" {
		t.Fatalf("node b model = %+v", b.Model)
	}

	c := plan.Node("c")
	if c.Harness != "" || c.Model != nil {
		t.Fatalf("command node carries an agent setting: %+v", c)
	}
	if c.Timeout.Duration() != 5*time.Minute {
		t.Fatalf("command timeout = %s", c.Timeout)
	}

	d := plan.Node("d")
	if d.Model == nil || d.Model.ID != "cheap" || d.Model.Thinking != "high" {
		t.Fatalf("node d model = %+v", d.Model)
	}

	if got := strings.Join(plan.Order, ","); got != "a,b,c,d" {
		t.Fatalf("order = %s", got)
	}
}

func TestNormalizeWorkflowResolvesSkillToAbsolutePath(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "openspec-explore")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	workflow := &Workflow{
		ID:    "skill",
		Nodes: []Node{{ID: "explore", Kind: NodeKindAgent, Harness: "pi", Skill: "openspec-explore"}},
	}
	plan, err := NormalizeWorkflow(workflow, NormalizeContext{SkillRoots: []string{root}})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	skill := plan.Node("explore").Skill
	if skill == nil || skill.Name != "openspec-explore" {
		t.Fatalf("skill = %+v", skill)
	}
	if len(skill.Paths) != 1 || skill.Paths[0] != skillDir {
		t.Fatalf("paths = %v, want %q", skill.Paths, skillDir)
	}
}

func TestNormalizeWorkflowReportsEveryMissingSkill(t *testing.T) {
	root := t.TempDir()
	workflow := &Workflow{
		ID: "missing",
		Nodes: []Node{
			{ID: "explore", Kind: NodeKindAgent, Harness: "pi", Skill: "missing-one"},
			{ID: "apply", Kind: NodeKindAgent, Harness: "pi", Skill: "missing-two"},
		},
	}

	_, err := NormalizeWorkflow(workflow, NormalizeContext{SkillRoots: []string{root}})
	if err == nil {
		t.Fatal("expected missing skills to fail normalization")
	}
	message := err.Error()
	for _, fragment := range []string{"explore", "missing-one", "apply", "missing-two", root} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("error %q does not mention %q", message, fragment)
		}
	}
}

func TestCompileWorkflowRejectsStructuralDefects(t *testing.T) {
	source := `
id: broken
nodes:
  - {id: a, kind: command, command: "true"}
  - {id: a, kind: command, command: "true"}
`
	_, err := CompileWorkflow([]byte(source), "broken.yaml", NormalizeContext{})
	if err == nil {
		t.Fatal("expected a duplicate identifier to fail compilation")
	}
	if !strings.Contains(err.Error(), "duplicate node id") {
		t.Fatalf("error = %v", err)
	}
}

func TestCompileWorkflowAcceptsAnUnknownModel(t *testing.T) {
	source := `
id: model
defaults: {harness: pi}
nodes:
  - {id: a, kind: agent, model: made-up-model}
`
	plan, err := CompileWorkflow([]byte(source), "model.yaml", NormalizeContext{})
	if err != nil {
		t.Fatalf("compile rejected a model identifier: %v", err)
	}
	if plan.Node("a").Model == nil || plan.Node("a").Model.ID != "made-up-model" {
		t.Fatalf("model = %+v", plan.Node("a").Model)
	}
}

func TestCompileWorkflowCanonicalTemplate(t *testing.T) {
	path := filepath.Join("..", "..", "..", "workflows", "standard-feature.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical workflow: %v", err)
	}

	skillRoot := t.TempDir()
	for _, name := range []string{"openspec-explore", "openspec-propose", "openspec-apply-change", "openspec-archive-change"} {
		if err := os.MkdirAll(filepath.Join(skillRoot, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := CompileWorkflow(data, path, NormalizeContext{SkillRoots: []string{skillRoot}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(plan.Nodes) != 10 {
		t.Fatalf("nodes = %d", len(plan.Nodes))
	}

	explore := plan.Node("explore")
	if explore == nil || explore.Model == nil || explore.Model.ID != "" || explore.Model.Thinking != "high" {
		t.Fatalf("explore model = %+v", explore)
	}
	if explore.Skill == nil || len(explore.Skill.Paths) != 1 {
		t.Fatalf("explore skill = %+v", explore.Skill)
	}

	verify := plan.Node("verify")
	if verify == nil || verify.Kind != NodeKindCommand || verify.Harness != "" || verify.Timeout.Duration() != 10*time.Minute {
		t.Fatalf("verify = %+v", verify)
	}

	merge := plan.Node("merge")
	if merge == nil || merge.Workdir != WorkdirProject || !merge.RequireApproval {
		t.Fatalf("merge = %+v", merge)
	}

	gate := plan.Node("review_gate")
	if gate == nil || gate.Routes[0] != "clean" || gate.Routes[1] != "issues" {
		t.Fatalf("review_gate = %+v", gate)
	}
	conditional := 0
	for _, edge := range plan.Edges {
		if edge.From != "review_gate" {
			continue
		}
		conditional++
	}
	if conditional != 2 {
		t.Fatalf("review_gate edges = %d, want 2", conditional)
	}
}

func TestCompileWorkflowKeepsTheSourceLabel(t *testing.T) {
	source := `
id: label
nodes:
  - {id: a, kind: command, command: "true"}
`
	plan, err := CompileWorkflow([]byte(source), "/tmp/known.yaml", NormalizeContext{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if plan.Source != "/tmp/known.yaml" || plan.ID != "label" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestNormalizeWorkflowResolvesInteractionTimeouts(t *testing.T) {
	tests := []struct {
		name         string
		defaults     Defaults
		node         Node
		wantAsk      time.Duration
		wantApproval time.Duration
	}{
		{
			name:         "node overrides the workflow default",
			defaults:     Defaults{Harness: "fake", AskTimeout: "30m", ApprovalTimeout: "40m"},
			node:         Node{ID: "a", Kind: NodeKindAgent, AskTimeout: "5m", ApprovalTimeout: "7m"},
			wantAsk:      5 * time.Minute,
			wantApproval: 7 * time.Minute,
		},
		{
			name:         "workflow default applies",
			defaults:     Defaults{Harness: "fake", AskTimeout: "10m", ApprovalTimeout: "20m"},
			node:         Node{ID: "a", Kind: NodeKindAgent},
			wantAsk:      10 * time.Minute,
			wantApproval: 20 * time.Minute,
		},
		{
			name:     "nothing configured",
			defaults: Defaults{Harness: "fake"},
			node:     Node{ID: "a", Kind: NodeKindAgent},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := &Workflow{ID: "timeouts", Defaults: test.defaults, Nodes: []Node{test.node}}
			plan, err := NormalizeWorkflow(workflow, NormalizeContext{})
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			node := plan.Node("a")
			if node.AskTimeout.Duration() != test.wantAsk {
				t.Fatalf("ask timeout = %s, want %s", node.AskTimeout, test.wantAsk)
			}
			if node.ApprovalTimeout.Duration() != test.wantApproval {
				t.Fatalf("approval timeout = %s, want %s", node.ApprovalTimeout, test.wantApproval)
			}
		})
	}
}

func TestValidateWorkflowRejectsMalformedInteractionTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		contains string
	}{
		{
			name: "defaults ask timeout",
			source: `
id: broken
defaults: {harness: fake, ask_timeout: soon}
nodes:
  - {id: a, kind: agent, prompt: work}
`,
			contains: `ask_timeout "soon"`,
		},
		{
			name: "defaults approval timeout",
			source: `
id: broken
defaults: {harness: fake, approval_timeout: later}
nodes:
  - {id: a, kind: agent, prompt: work}
`,
			contains: `approval_timeout "later"`,
		},
		{
			name: "node ask timeout",
			source: `
id: broken
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work, ask_timeout: shortly}
`,
			contains: `node "a" has invalid ask_timeout "shortly"`,
		},
		{
			name: "node approval timeout",
			source: `
id: broken
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, prompt: work, approval_timeout: eventually}
`,
			contains: `node "a" has invalid approval_timeout "eventually"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow, err := ParseWorkflow([]byte(test.source), "broken.yaml")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = ValidateWorkflow(workflow)
			if err == nil {
				t.Fatal("expected the malformed timeout to be rejected")
			}
			if !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want it to contain %q", err, test.contains)
			}
		})
	}
}

func TestNormalizeWorkflowResolvesMultipleSkillsInOrder(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	workflow := &Workflow{
		ID: "skills",
		Nodes: []Node{{
			ID: "explore", Kind: NodeKindAgent, Harness: "pi",
			Skills: []string{"alpha", "beta"},
		}},
	}
	plan, err := NormalizeWorkflow(workflow, NormalizeContext{SkillRoots: []string{root}})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	node := plan.Node("explore")
	if len(node.Skills) != 2 {
		t.Fatalf("skills = %+v, want two bindings", node.Skills)
	}
	if node.Skills[0].Name != "alpha" || node.Skills[1].Name != "beta" {
		t.Fatalf("skill order = %+v", node.Skills)
	}
	if node.Skill == nil || node.Skill.Name != "alpha" {
		t.Fatalf("legacy first skill = %+v", node.Skill)
	}
}

func TestValidateWorkflowRejectsBothSkillForms(t *testing.T) {
	workflow := &Workflow{
		ID: "skills",
		Nodes: []Node{{
			ID: "explore", Kind: NodeKindAgent, Harness: "pi",
			Skill: "alpha", Skills: []string{"beta"},
		}},
	}
	err := ValidateWorkflow(workflow)
	if err == nil || !strings.Contains(err.Error(), "both skill and skills") {
		t.Fatalf("error = %v, want a both-forms problem", err)
	}
}

func TestCompileWorkflowCarriesEdgesAndRoutes(t *testing.T) {
	source := `
id: routed
defaults: {harness: fake}
nodes:
  - {id: gate, kind: command, command: "test -s findings.md", routes: {1: issues, 0: clean}}
  - {id: fix, kind: agent, prompt: fix it}
  - {id: verify, kind: command, command: "true"}
edges:
  - {from: gate, to: fix, when: issues}
  - {from: gate, to: verify, when: clean}
`
	plan, err := CompileWorkflow([]byte(source), "routed.yaml", NormalizeContext{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	want := []PlanEdge{
		{From: "gate", To: "fix", When: "issues"},
		{From: "gate", To: "verify", When: "clean"},
	}
	if !reflect.DeepEqual(plan.Edges, want) {
		t.Fatalf("plan edges = %+v, want %+v", plan.Edges, want)
	}

	gate := plan.Node("gate")
	if gate == nil {
		t.Fatal("plan lost the gate")
	}
	if gate.Routes[0] != "clean" || gate.Routes[1] != "issues" {
		t.Fatalf("gate routes = %+v", gate.Routes)
	}

	outgoing := plan.Outgoing("gate")
	if len(outgoing) != 2 || outgoing[0].To != "fix" || outgoing[1].To != "verify" {
		t.Fatalf("outgoing = %+v", outgoing)
	}
	if fixOut := plan.Outgoing("fix"); len(fixOut) != 0 {
		t.Fatalf("fix has outgoing edges: %+v", fixOut)
	}
}
