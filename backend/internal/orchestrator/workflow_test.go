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
	if len(workflow.Nodes) != 8 {
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

			err := ValidateWorkflow(workflow)
			if len(test.contains) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, fragment := range test.contains {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error %q does not contain %q", err, fragment)
				}
			}
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
