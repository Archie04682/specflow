package orchestrator

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fullWorkflowDraft exercises every field the draft model carries, with both
// tri-state booleans declared so conversion cannot collapse them.
func fullWorkflowDraft() WorkflowDraft {
	return WorkflowDraft{
		ID:          "standard-feature",
		Name:        "Standard feature",
		Description: "Explore, propose, apply, review, test, verify, archive, merge.",
		Version:     2,
		Defaults: WorkflowDefaultsDraft{
			Harness:         "pi",
			Model:           &ModelDraft{ID: "default", Thinking: "high"},
			Interactive:     boolPointer(false),
			AllowInject:     boolPointer(true),
			AskTimeout:      "30m",
			ApprovalTimeout: "1h",
		},
		Nodes: []WorkflowNodeDraft{
			{
				ID:              "explore",
				Title:           "Explore",
				Kind:            NodeKindAgent,
				Harness:         "pi",
				Skill:           "openspec-explore",
				Prompt:          "The run objective is: {{objective}}\nExplore it and ask when needed.",
				Workdir:         WorkdirRun,
				Model:           &ModelDraft{ID: "default", Thinking: "high"},
				Interactive:     boolPointer(true),
				AllowInject:     boolPointer(true),
				RequireApproval: false,
				Timeout:         "30m",
				AskTimeout:      "10m",
				ApprovalTimeout: "1h",
				Retry:           1,
				Tools:           []string{"read", "write"},
				Postcondition: &Postcondition{
					Type:        "new_change",
					Change:      "{{change}}",
					Description: "Propose must create a change",
				},
			},
			{
				ID:              "verify",
				Title:           "Verify",
				Kind:            NodeKindCommand,
				Command:         `openspec validate "{{change}}" --strict`,
				Workdir:         WorkdirProject,
				RequireApproval: true,
				Interactive:     boolPointer(false),
				AllowInject:     boolPointer(false),
				Timeout:         "10m",
				Postcondition:   &Postcondition{Type: "exit_zero"},
			},
		},
		Edges: []WorkflowEdgeDraft{{From: "explore", To: "verify"}},
	}
}

func TestDraftFromWorkflowEmpty(t *testing.T) {
	draft := draftFromWorkflow(&Workflow{})

	if draft.Nodes == nil || draft.Edges == nil {
		t.Fatalf("empty workflow draft has nil arrays: %+v", draft)
	}
	if len(draft.Nodes) != 0 || len(draft.Edges) != 0 {
		t.Fatalf("empty workflow draft has content: %+v", draft)
	}

	encoded, err := json.Marshal(draft)
	if err != nil {
		t.Fatalf("encode draft: %v", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	for _, key := range []string{"id", "defaults", "nodes", "edges"} {
		if _, ok := object[key]; !ok {
			t.Fatalf("draft JSON has no %q: %s", key, encoded)
		}
	}
	if got := string(object["defaults"]); got != "{}" {
		t.Fatalf("defaults = %s, want an object", got)
	}
	if got := string(object["nodes"]); got != "[]" {
		t.Fatalf("nodes = %s, want an array", got)
	}
	if got := string(object["edges"]); got != "[]" {
		t.Fatalf("edges = %s, want an array", got)
	}
	for _, omitted := range []string{"name", "description", "version"} {
		if _, ok := object[omitted]; ok {
			t.Fatalf("empty %q was not omitted: %s", omitted, encoded)
		}
	}
}

func TestMarshalWorkflowYAMLMinimalRoundTrip(t *testing.T) {
	input := WorkflowDraft{
		ID:       "minimal",
		Defaults: WorkflowDefaultsDraft{Harness: "pi"},
		Nodes: []WorkflowNodeDraft{{
			ID: "only", Kind: NodeKindAgent, Model: &ModelDraft{},
		}},
	}

	data, err := MarshalWorkflowYAML(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	workflow, err := ParseWorkflow(data, "minimal.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("generated minimal workflow is invalid: %v", err)
	}

	// A nil edge list and an empty model normalize to their canonical empty
	// forms, so compare against the conversion rather than the literal input.
	want := draftFromWorkflow(workflowFromDraft(input))
	if got := draftFromWorkflow(workflow); !reflect.DeepEqual(got, want) {
		t.Fatalf("minimal draft did not survive the round trip:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestWorkflowDraftJSONAlwaysUsesArrays(t *testing.T) {
	encoded, err := json.Marshal(WorkflowDraft{})
	if err != nil {
		t.Fatalf("encode draft: %v", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if got := string(object["nodes"]); got != "[]" {
		t.Fatalf("nodes = %s, want an array: %s", got, encoded)
	}
	if got := string(object["edges"]); got != "[]" {
		t.Fatalf("edges = %s, want an array: %s", got, encoded)
	}
	if got := string(object["defaults"]); got != "{}" {
		t.Fatalf("defaults = %s, want an object: %s", got, encoded)
	}
}

func TestDraftJSONKeepsTriStateBooleans(t *testing.T) {
	draft := WorkflowDraft{
		ID: "json",
		Defaults: WorkflowDefaultsDraft{
			Interactive: boolPointer(false),
		},
		Nodes: []WorkflowNodeDraft{
			{ID: "unset", Kind: NodeKindAgent},
			{ID: "false", Kind: NodeKindAgent, Interactive: boolPointer(false)},
		},
		Edges: []WorkflowEdgeDraft{},
	}

	encoded, err := json.Marshal(draft)
	if err != nil {
		t.Fatalf("encode draft: %v", err)
	}
	if !strings.Contains(string(encoded), `"interactive":false`) {
		t.Fatalf("an explicit false was omitted: %s", encoded)
	}

	var decoded WorkflowDraft
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if decoded.Nodes[0].Interactive != nil {
		t.Fatalf("an absent boolean became set: %+v", decoded.Nodes[0])
	}
	if decoded.Nodes[1].Interactive == nil || *decoded.Nodes[1].Interactive {
		t.Fatalf("an explicit false was lost: %+v", decoded.Nodes[1])
	}
	if decoded.Defaults.Interactive == nil || *decoded.Defaults.Interactive {
		t.Fatalf("defaults false was lost: %+v", decoded.Defaults)
	}
}

func TestDraftConversionRoundTripsAFullWorkflow(t *testing.T) {
	draft := fullWorkflowDraft()

	workflow := workflowFromDraft(draft)
	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("converted workflow is invalid: %v", err)
	}
	if workflow.ID != draft.ID || workflow.Name != draft.Name || workflow.Version != draft.Version {
		t.Fatalf("workflow metadata = %+v", workflow)
	}
	if workflow.Defaults.Harness != "pi" || workflow.Defaults.Model.ID != "default" {
		t.Fatalf("workflow defaults = %+v", workflow.Defaults)
	}
	explore := workflow.Node("explore")
	if explore == nil {
		t.Fatal("converted workflow lost the explore node")
	}
	if explore.Model.Thinking != "high" || explore.Retry != 1 || len(explore.Tools) != 2 {
		t.Fatalf("explore node = %+v", explore)
	}
	if explore.Postcondition == nil || explore.Postcondition.Type != "new_change" {
		t.Fatalf("explore postcondition = %+v", explore.Postcondition)
	}

	if converted := draftFromWorkflow(workflow); !reflect.DeepEqual(converted, draft) {
		t.Fatalf("draft did not survive conversion:\n got: %+v\nwant: %+v", converted, draft)
	}
}

func TestDraftKeepsTriStateBooleans(t *testing.T) {
	draft := WorkflowDraft{
		ID:       "tri-state",
		Defaults: WorkflowDefaultsDraft{Harness: "pi", Interactive: boolPointer(false)},
		Nodes: []WorkflowNodeDraft{
			{ID: "inherit", Kind: NodeKindAgent},
			{ID: "off", Kind: NodeKindAgent, Interactive: boolPointer(false), AllowInject: boolPointer(false)},
			{ID: "on", Kind: NodeKindAgent, Interactive: boolPointer(true), AllowInject: boolPointer(true)},
		},
		Edges: []WorkflowEdgeDraft{},
	}

	workflow := workflowFromDraft(draft)
	if workflow.Defaults.Interactive == nil || *workflow.Defaults.Interactive {
		t.Fatalf("defaults interactive = %+v, want false", workflow.Defaults.Interactive)
	}
	if workflow.Nodes[0].Interactive != nil || workflow.Nodes[0].AllowInject != nil {
		t.Fatalf("unset node booleans became set: %+v", workflow.Nodes[0])
	}
	if workflow.Nodes[1].Interactive == nil || *workflow.Nodes[1].Interactive {
		t.Fatalf("false node boolean was lost: %+v", workflow.Nodes[1])
	}
	if workflow.Nodes[2].Interactive == nil || !*workflow.Nodes[2].Interactive {
		t.Fatalf("true node boolean was lost: %+v", workflow.Nodes[2])
	}

	// Effective values still resolve against the workflow defaults.
	if workflow.EffectiveInteractive(&workflow.Nodes[0]) {
		t.Fatal("an unset node inherited true instead of the default false")
	}

	if converted := draftFromWorkflow(workflow); !reflect.DeepEqual(converted, draft) {
		t.Fatalf("tri-state draft did not survive conversion:\n got: %+v\nwant: %+v", converted, draft)
	}
}

func TestModelDraftEncodesScalarAndMapping(t *testing.T) {
	scalar := WorkflowDraft{
		ID: "models",
		Nodes: []WorkflowNodeDraft{{
			ID: "a", Kind: NodeKindAgent, Harness: "pi", Model: &ModelDraft{ID: "default"},
		}},
		Edges: []WorkflowEdgeDraft{},
	}
	data, err := MarshalWorkflowYAML(scalar)
	if err != nil {
		t.Fatalf("marshal scalar model: %v", err)
	}
	if !strings.Contains(string(data), "model: default") {
		t.Fatalf("scalar model was not rendered as a scalar:\n%s", data)
	}
	if strings.Contains(string(data), "thinking:") {
		t.Fatalf("scalar model gained a thinking field:\n%s", data)
	}

	workflow, err := ParseWorkflow(data, "scalar.yaml")
	if err != nil {
		t.Fatalf("parse scalar model: %v", err)
	}
	if got := workflow.Nodes[0].Model; got.ID != "default" || got.Thinking != "" {
		t.Fatalf("parsed scalar model = %+v", got)
	}

	mapping := scalar
	mapping.Nodes = []WorkflowNodeDraft{{
		ID: "a", Kind: NodeKindAgent, Harness: "pi",
		Model: &ModelDraft{ID: "deepseek-chat", Thinking: "high"},
	}}
	data, err = MarshalWorkflowYAML(mapping)
	if err != nil {
		t.Fatalf("marshal mapping model: %v", err)
	}
	if !strings.Contains(string(data), "thinking: high") {
		t.Fatalf("model reasoning effort was not rendered:\n%s", data)
	}
	workflow, err = ParseWorkflow(data, "mapping.yaml")
	if err != nil {
		t.Fatalf("parse mapping model: %v", err)
	}
	if got := workflow.Nodes[0].Model; got.ID != "deepseek-chat" || got.Thinking != "high" {
		t.Fatalf("parsed mapping model = %+v", got)
	}
}

func TestMarshalWorkflowYAMLHasTheManagedHeader(t *testing.T) {
	data, err := MarshalWorkflowYAML(fullWorkflowDraft())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	want := "# Managed by the SpecFlow workflow editor.\n" +
		"# Comments and formatting are not preserved on save.\n\n"
	if !strings.HasPrefix(string(data), want) {
		t.Fatalf("canonical YAML does not start with the managed header:\n%s", data)
	}
	if strings.HasPrefix(strings.TrimPrefix(string(data), want), "#") {
		t.Fatalf("canonical YAML has unexpected leading comments:\n%s", data)
	}
}

func TestMarshalWorkflowYAMLUsesSnakeCaseKeys(t *testing.T) {
	draft := WorkflowDraft{
		ID: "keys",
		Defaults: WorkflowDefaultsDraft{
			Harness:         "pi",
			AllowInject:     boolPointer(true),
			AskTimeout:      "30m",
			ApprovalTimeout: "1h",
		},
		Nodes: []WorkflowNodeDraft{{
			ID:              "node",
			Kind:            NodeKindAgent,
			Harness:         "pi",
			Interactive:     boolPointer(false),
			AllowInject:     boolPointer(true),
			RequireApproval: true,
			AskTimeout:      "10m",
			ApprovalTimeout: "20m",
		}},
		Edges: []WorkflowEdgeDraft{},
	}

	data, err := MarshalWorkflowYAML(draft)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"allow_inject:", "ask_timeout:", "approval_timeout:", "require_approval:"} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("canonical YAML has no %q key:\n%s", key, data)
		}
	}
	for _, key := range []string{"allowInject:", "askTimeout:", "approvalTimeout:", "requireApproval:"} {
		if strings.Contains(string(data), key) {
			t.Fatalf("canonical YAML leaks camelCase key %q:\n%s", key, data)
		}
	}
}

func TestMarshalWorkflowYAMLEmptyWorkflow(t *testing.T) {
	data, err := MarshalWorkflowYAML(WorkflowDraft{ID: "blank"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)

	if !strings.Contains(text, "nodes: []") {
		t.Fatalf("empty nodes are not rendered as an array:\n%s", text)
	}
	for _, omitted := range []string{"defaults:", "edges:", "name:", "description:", "version:"} {
		if strings.Contains(text, omitted) {
			t.Fatalf("empty workflow rendered %q:\n%s", omitted, text)
		}
	}
}

func TestMarshalWorkflowYAMLIsCanonicalAndRoundTrips(t *testing.T) {
	draft := fullWorkflowDraft()

	data, err := MarshalWorkflowYAML(draft)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// The same draft must serialize identically: the file is canonical.
	again, err := MarshalWorkflowYAML(draft)
	if err != nil {
		t.Fatalf("marshal again: %v", err)
	}
	if string(data) != string(again) {
		t.Fatalf("canonical YAML is not deterministic:\n%s\n---\n%s", data, again)
	}

	workflow, err := ParseWorkflow(data, "generated.yaml")
	if err != nil {
		t.Fatalf("generated YAML does not parse: %v", err)
	}
	if err := ValidateWorkflow(workflow); err != nil {
		t.Fatalf("generated YAML is invalid: %v", err)
	}
	if converted := draftFromWorkflow(workflow); !reflect.DeepEqual(converted, draft) {
		t.Fatalf("draft did not survive the YAML round trip:\n got: %+v\nwant: %+v", converted, draft)
	}
}

func TestWorkflowFromDraftRejectsAnEmptyGraph(t *testing.T) {
	workflow := workflowFromDraft(WorkflowDraft{ID: "empty"})
	err := ValidateWorkflow(workflow)
	if err == nil {
		t.Fatal("an empty draft was accepted")
	}
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("empty workflow error = %T, want *ValidationError", err)
	}
	if len(validation.Problems) == 0 {
		t.Fatal("empty workflow validation reported no problems")
	}
}

func TestDraftCarriesMultipleSkillsAndMarshalsThemAsAList(t *testing.T) {
	draft := WorkflowDraft{
		ID: "skills",
		Nodes: []WorkflowNodeDraft{{
			ID: "explore", Kind: NodeKindAgent, Harness: "pi",
			Skills: []string{"alpha", "beta"},
			Prompt: "do it",
		}},
	}

	encoded, err := MarshalWorkflowYAML(draft)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(encoded)
	if !strings.Contains(text, "skills:") || strings.Contains(text, "\n    skill:") {
		t.Fatalf("canonical YAML did not use the skills list:\n%s", text)
	}

	workflow, err := ParseWorkflow(encoded, "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	skills := workflow.Nodes[0].EffectiveSkills()
	if len(skills) != 2 || skills[0] != "alpha" || skills[1] != "beta" {
		t.Fatalf("skills = %#v", skills)
	}

	roundTrip := draftFromWorkflow(workflow)
	if len(roundTrip.Nodes[0].Skills) != 2 {
		t.Fatalf("draft skills = %#v", roundTrip.Nodes[0].Skills)
	}
}
