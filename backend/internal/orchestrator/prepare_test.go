package orchestrator

import (
	"strings"
	"testing"

	"example.com/specflow/backend/internal/harness"
)

func TestRenderPromptSubstitutesTheClosedSet(t *testing.T) {
	context := RunContext{Objective: "add export", Change: "add-export", RunID: "run-1", Workdir: "/tmp/wt"}

	rendered, err := RenderPrompt("Goal: {{objective}}\nChange: {{change}}\nRun {{run_id}} in {{workdir}}", context)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, fragment := range []string{"add export", "add-export", "run-1", "/tmp/wt"} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered prompt %q lacks %q", rendered, fragment)
		}
	}
}

func TestRenderPromptRejectsUnsupportedPlaceholders(t *testing.T) {
	_, err := RenderPrompt("Plan: {{planner.steps}}", RunContext{})
	if err == nil {
		t.Fatal("expected an unsupported placeholder to fail")
	}
	if !strings.Contains(err.Error(), "planner.steps") {
		t.Fatalf("error does not name the placeholder: %v", err)
	}
}

func TestRenderPromptLeavesPlainTextAlone(t *testing.T) {
	rendered, err := RenderPrompt("Review current changes", RunContext{})
	if err != nil {
		t.Fatal(err)
	}
	if rendered != "Review current changes" {
		t.Fatalf("rendered = %q", rendered)
	}
}

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name     string
		node     ModelValue
		defaults ModelValue
		wantID   string
		wantEff  string
	}{
		{
			name:     "node overrides the workflow default",
			node:     ModelValue{ID: "coder", Thinking: "high"},
			defaults: ModelValue{ID: "cheap", Thinking: "low"},
			wantID:   "coder",
			wantEff:  "high",
		},
		{
			name:     "workflow default applies when the node is silent",
			defaults: ModelValue{ID: "cheap", Thinking: "low"},
			wantID:   "cheap",
			wantEff:  "low",
		},
		{
			name:     "empty means omit",
			defaults: ModelValue{},
		},
		{
			name:     "the default literal is never forwarded",
			node:     ModelValue{ID: "default", Thinking: "default"},
			defaults: ModelValue{ID: "default", Thinking: "default"},
		},
		{
			name:     "an explicit node default stops the fallback",
			node:     ModelValue{ID: "default"},
			defaults: ModelValue{ID: "cheap"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := ResolveModel(&Node{Model: test.node}, Defaults{Model: test.defaults})
			if model.ID != test.wantID {
				t.Fatalf("id = %q, want %q", model.ID, test.wantID)
			}
			if model.Thinking != test.wantEff {
				t.Fatalf("thinking = %q, want %q", model.Thinking, test.wantEff)
			}
		})
	}
}

func TestResolveWorkdir(t *testing.T) {
	tests := []struct {
		workdir string
		want    string
		wantErr bool
	}{
		{workdir: "", want: "/wt"},
		{workdir: WorkdirRun, want: "/wt"},
		{workdir: WorkdirProject, want: "/project"},
		{workdir: "elsewhere", wantErr: true},
	}

	for _, test := range tests {
		workdir, err := ResolveWorkdir(&PlanNode{ID: "n", Workdir: test.workdir}, "/wt", "/project")
		if test.wantErr {
			if err == nil {
				t.Fatalf("%q: expected an error", test.workdir)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", test.workdir, err)
		}
		if workdir != test.want {
			t.Fatalf("%q -> %q, want %q", test.workdir, workdir, test.want)
		}
	}
}

func TestPreflightRefusesAnUnmetRequirement(t *testing.T) {
	node := &PlanNode{
		ID:          "explore",
		Kind:        NodeKindAgent,
		Interactive: true,
		Skill:       &harness.Skill{Name: "openspec-explore"},
	}

	err := Preflight(node, harness.Capabilities{})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	message := err.Error()
	for _, fragment := range []string{"explore", "questions", "skills"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("refusal %q does not mention %q", message, fragment)
		}
	}
}

func TestPreflightAcceptsSatisfiedRequirements(t *testing.T) {
	node := &PlanNode{
		ID:          "explore",
		Kind:        NodeKindAgent,
		Interactive: true,
		Skill:       &harness.Skill{Name: "openspec-explore"},
	}
	capabilities := harness.Capabilities{Questions: true, Skills: true}

	if err := Preflight(node, capabilities); err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
