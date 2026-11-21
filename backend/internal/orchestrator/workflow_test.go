package orchestrator

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func validWorkflow() *Workflow {
	return &Workflow{
		ID: "test",
		Nodes: []Node{
			{ID: "a", Kind: NodeKindCommand, Command: "true"},
			{ID: "b", Kind: NodeKindCommand, Command: "true"},
		},
		Edges: []Edge{{From: "a", To: "b"}},
	}
}

func TestLoadCanonicalWorkflow(t *testing.T) {
	path := filepath.Join("..", "..", "..", "workflows", "standard-feature.yaml")

	workflow, err := LoadWorkflow(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if workflow.ID != "standard-feature" {
		t.Fatalf("id = %q", workflow.ID)
	}
	if len(workflow.Nodes) != 10 {
		t.Fatalf("nodes = %d", len(workflow.Nodes))
	}
	if workflow.Nodes[0].ID != "explore" {
		t.Fatalf("first node = %q", workflow.Nodes[0].ID)
	}

	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("canonical workflow is invalid: %v", err)
	}

	merge := workflow.Node("merge")
	if merge == nil {
		t.Fatal("canonical workflow has no merge node")
	}
	if merge.Kind != NodeKindCommand || merge.Workdir != WorkdirProject || !merge.RequireApproval {
		t.Fatalf("merge node = %+v", merge)
	}

	verify := workflow.Node("verify")
	if verify == nil || verify.Kind != NodeKindCommand || verify.Harness != "" {
		t.Fatalf("verify must be a command gate with no harness: %+v", verify)
	}

	// The canonical review path is the worked example of conditional routing.
	gate := workflow.Node("review_gate")
	if gate == nil || gate.Kind != NodeKindCommand {
		t.Fatalf("review_gate = %+v", gate)
	}
	if gate.Routes[0] != "clean" || gate.Routes[1] != "issues" {
		t.Fatalf("review_gate routes = %+v", gate.Routes)
	}
	if fix := workflow.Node("fix"); fix == nil || fix.Kind != NodeKindAgent {
		t.Fatalf("fix = %+v", fix)
	}
	wants := map[string]string{"fix": "issues", "tests": "clean"}
	for _, edge := range workflow.Edges {
		if edge.From != "review_gate" {
			continue
		}
		if wants[edge.To] != edge.When {
			t.Fatalf("review_gate edge to %q has when %q, want %q", edge.To, edge.When, wants[edge.To])
		}
		delete(wants, edge.To)
	}
	if len(wants) != 0 {
		t.Fatalf("review_gate is missing conditional edges: %+v", wants)
	}
}

func TestValidateWorkflow(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Workflow)
		contains []string
	}{
		{
			name:   "valid",
			mutate: func(*Workflow) {},
		},
		{
			name: "duplicate identifier",
			mutate: func(w *Workflow) {
				w.Nodes[1].ID = "a"
				w.Edges = nil
			},
			contains: []string{"duplicate node id"},
		},
		{
			name: "edge to missing node",
			mutate: func(w *Workflow) {
				w.Edges = append(w.Edges, Edge{From: "a", To: "ghost"})
			},
			contains: []string{`unknown node "ghost"`},
		},
		{
			name: "cycle",
			mutate: func(w *Workflow) {
				w.Edges = append(w.Edges, Edge{From: "b", To: "a"})
			},
			contains: []string{"cycle"},
		},
		{
			name: "unknown kind",
			mutate: func(w *Workflow) {
				w.Nodes[0].Kind = "magic"
			},
			contains: []string{"unknown kind"},
		},
		{
			name: "agent without a harness",
			mutate: func(w *Workflow) {
				w.Nodes[0] = Node{ID: "a", Kind: NodeKindAgent}
			},
			contains: []string{"names no harness"},
		},
		{
			name: "command without a command",
			mutate: func(w *Workflow) {
				w.Nodes[0].Command = ""
			},
			contains: []string{"declares no command"},
		},
		{
			name: "two defects are reported together",
			mutate: func(w *Workflow) {
				w.Nodes[1].ID = "a"
				w.Edges = append(w.Edges, Edge{From: "a", To: "ghost"})
			},
			contains: []string{"duplicate node id", `unknown node "ghost"`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := validWorkflow()
			test.mutate(workflow)
			assertValidation(t, workflow, test.contains)
		})
	}
}

func TestWorkflowOrderIsStable(t *testing.T) {
	workflow := &Workflow{
		Nodes: []Node{
			{ID: "c", Kind: NodeKindCommand, Command: "true"},
			{ID: "a", Kind: NodeKindCommand, Command: "true"},
			{ID: "b", Kind: NodeKindCommand, Command: "true"},
		},
		Edges: []Edge{{From: "a", To: "c"}, {From: "b", To: "c"}},
	}

	order, err := workflow.Order()
	if err != nil {
		t.Fatal(err)
	}

	// Declaration order breaks ties: c is blocked, a comes before b.
	want := []string{"a", "b", "c"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestModelValueAcceptsScalarAndMapping(t *testing.T) {
	var scalar ModelValue
	if err := scalar.UnmarshalYAML(yamlScalar("default")); err != nil {
		t.Fatal(err)
	}
	if scalar.ID != "default" || scalar.Thinking != "" {
		t.Fatalf("scalar = %+v", scalar)
	}

	var mapping ModelValue
	if err := mapping.UnmarshalYAML(yamlMapping(map[string]string{"id": "deepseek-chat", "thinking": "high"})); err != nil {
		t.Fatal(err)
	}
	if mapping.ID != "deepseek-chat" || mapping.Thinking != "high" {
		t.Fatalf("mapping = %+v", mapping)
	}
}

func yamlScalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}

func yamlMapping(values map[string]string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	for key, value := range values {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Value: value},
		)
	}
	return node
}

// assertValidation runs one workflow past the validator: the named problems must
// be reported, or the workflow must be accepted when none are named.
func assertValidation(t *testing.T, workflow *Workflow, contains []string) {
	t.Helper()

	err := ValidateWorkflow(workflow)
	if len(contains) == 0 {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, fragment := range contains {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not contain %q", err, fragment)
		}
	}
}

// routingWorkflow is a valid diamond: an agent reviews, a command gate routes
// on its exit code, and both branches reach verify.
func routingWorkflow() *Workflow {
	return &Workflow{
		Nodes: []Node{
			{ID: "review", Kind: NodeKindAgent, Harness: "pi"},
			{ID: "gate", Kind: NodeKindCommand, Command: "test ! -s findings.md", Routes: map[int]string{0: "clean", 1: "issues"}},
			{ID: "fix", Kind: NodeKindAgent, Harness: "pi"},
			{ID: "verify", Kind: NodeKindCommand, Command: "true"},
		},
		Edges: []Edge{
			{From: "review", To: "gate"},
			{From: "gate", To: "fix", When: "issues"},
			{From: "gate", To: "verify", When: "clean"},
			{From: "fix", To: "verify"},
		},
	}
}

func TestValidateWorkflowRouting(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Workflow)
		contains []string
	}{
		{
			name:   "valid diamond",
			mutate: func(*Workflow) {},
		},
		{
			name: "routes on a node that has no exit code",
			mutate: func(w *Workflow) {
				w.Nodes[0].Routes = map[int]string{0: "clean"}
			},
			contains: []string{`node "review" declares routes but is not a command node`},
		},
		{
			name: "routes and a postcondition",
			mutate: func(w *Workflow) {
				w.Nodes[1].Postcondition = &Postcondition{Type: "exit_zero"}
			},
			contains: []string{`node "gate" declares both routes and a postcondition`},
		},
		{
			name: "an unlabelled edge leaves a routing node",
			mutate: func(w *Workflow) {
				w.Edges[1].When = ""
			},
			contains: []string{`edge "gate" -> "fix" declares no when`},
		},
		{
			name: "an edge names a label the gate does not route",
			mutate: func(w *Workflow) {
				w.Edges[1].When = "major"
			},
			contains: []string{`declares when "major" which node "gate" does not route`},
		},
		{
			name: "one label selects two edges",
			mutate: func(w *Workflow) {
				w.Edges = append(w.Edges, Edge{From: "gate", To: "fix", When: "issues"})
			},
			contains: []string{`node "gate" routes "issues" on more than one outgoing edge`},
		},
		{
			name: "a declared label is never used",
			mutate: func(w *Workflow) {
				w.Nodes[1].Routes = map[int]string{0: "clean", 1: "issues", 2: "major"}
			},
			contains: []string{`node "gate" declares route "major" but no outgoing edge uses it`},
		},
		{
			name: "an empty label",
			mutate: func(w *Workflow) {
				w.Nodes[1].Routes = map[int]string{0: "clean", 1: " "}
			},
			contains: []string{`node "gate" declares an empty route label for exit code 1`},
		},
		{
			name: "a condition on a node that declares no routes",
			mutate: func(w *Workflow) {
				w.Edges[3].When = "always"
			},
			contains: []string{`edge "fix" -> "verify" declares when "always" but node "fix" declares no routes`},
		},
		{
			name: "routing defects join the other defects",
			mutate: func(w *Workflow) {
				w.Nodes[0].Kind = "magic"
				w.Edges[1].When = ""
			},
			contains: []string{"unknown kind", `declares no when`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := routingWorkflow()
			test.mutate(workflow)
			assertValidation(t, workflow, test.contains)
		})
	}
}

func TestWorkflowParsesRoutesAndConditions(t *testing.T) {
	source := `
id: routed
nodes:
  - {id: gate, kind: command, command: "true", routes: {1: issues, 0: clean}}
  - {id: fix, kind: command, command: "true"}
edges:
  - {from: gate, to: fix, when: issues}
`
	workflow, err := ParseWorkflow([]byte(source), "routed.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gate := workflow.Node("gate")
	if gate == nil {
		t.Fatal("gate is missing")
	}
	if gate.Routes[0] != "clean" || gate.Routes[1] != "issues" {
		t.Fatalf("routes = %+v", gate.Routes)
	}
	if workflow.Edges[0].When != "issues" {
		t.Fatalf("edge = %+v", workflow.Edges[0])
	}
	if codes := gate.RouteCodes(); len(codes) != 2 || codes[0] != 0 || codes[1] != 1 {
		t.Fatalf("route codes = %v, want ascending", codes)
	}
}
