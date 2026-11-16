package orchestrator

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// WorkflowDraft is the editable JSON form of a workflow. It mirrors the YAML
// schema with camelCase JSON names; the client never parses or serializes YAML.
// Every pointer field is tri-state: nil means "inherit or absent", so an unset
// boolean is never confused with false.
type WorkflowDraft struct {
	ID          string                `json:"id"`
	Name        string                `json:"name,omitempty"`
	Description string                `json:"description,omitempty"`
	Version     int                   `json:"version,omitempty"`
	Defaults    WorkflowDefaultsDraft `json:"defaults"`
	Nodes       []WorkflowNodeDraft   `json:"nodes"`
	Edges       []WorkflowEdgeDraft   `json:"edges"`
}

// WorkflowDefaultsDraft is the editable form of workflow-wide node settings.
type WorkflowDefaultsDraft struct {
	Harness         string      `json:"harness,omitempty"`
	Model           *ModelDraft `json:"model,omitempty"`
	Interactive     *bool       `json:"interactive,omitempty"`
	AllowInject     *bool       `json:"allowInject,omitempty"`
	AskTimeout      string      `json:"askTimeout,omitempty"`
	ApprovalTimeout string      `json:"approvalTimeout,omitempty"`
}

// WorkflowNodeDraft is the editable form of one graph node.
type WorkflowNodeDraft struct {
	ID              string         `json:"id"`
	Title           string         `json:"title,omitempty"`
	Kind            NodeKind       `json:"kind"`
	Harness         string         `json:"harness,omitempty"`
	Skill           string         `json:"skill,omitempty"`
	Skills          []string       `json:"skills,omitempty"`
	Prompt          string         `json:"prompt,omitempty"`
	Command         string         `json:"command,omitempty"`
	Workdir         string         `json:"workdir,omitempty"`
	Model           *ModelDraft    `json:"model,omitempty"`
	Interactive     *bool          `json:"interactive,omitempty"`
	AllowInject     *bool          `json:"allowInject,omitempty"`
	RequireApproval bool           `json:"requireApproval,omitempty"`
	Timeout         string         `json:"timeout,omitempty"`
	AskTimeout      string         `json:"askTimeout,omitempty"`
	ApprovalTimeout string         `json:"approvalTimeout,omitempty"`
	Retry           int            `json:"retry,omitempty"`
	Tools           []string       `json:"tools,omitempty"`
	Postcondition   *Postcondition `json:"postcondition,omitempty"`
}

// WorkflowEdgeDraft is one authored ordering edge.
type WorkflowEdgeDraft struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ModelDraft is a model selection: a bare identifier, or an identifier with a
// reasoning effort.
type ModelDraft struct {
	ID       string `json:"id,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

// MarshalJSON guarantees the draft's collection fields encode as arrays, never
// null, even when the caller built the struct with nil slices.
func (d WorkflowDraft) MarshalJSON() ([]byte, error) {
	type draftJSON WorkflowDraft
	normalized := draftJSON(d)
	if normalized.Nodes == nil {
		normalized.Nodes = []WorkflowNodeDraft{}
	}
	if normalized.Edges == nil {
		normalized.Edges = []WorkflowEdgeDraft{}
	}
	return json.Marshal(normalized)
}

// draftFromWorkflow converts a parsed workflow into its editable form. Nodes and
// edges are always non-nil and the tri-state pointers keep their exact state.
func draftFromWorkflow(workflow *Workflow) WorkflowDraft {
	draft := WorkflowDraft{
		ID:          workflow.ID,
		Name:        workflow.Name,
		Description: workflow.Description,
		Version:     workflow.Version,
		Defaults:    defaultsDraftFromWorkflow(workflow.Defaults),
		Nodes:       make([]WorkflowNodeDraft, 0, len(workflow.Nodes)),
		Edges:       make([]WorkflowEdgeDraft, 0, len(workflow.Edges)),
	}
	for _, node := range workflow.Nodes {
		draft.Nodes = append(draft.Nodes, nodeDraftFromWorkflow(node))
	}
	for _, edge := range workflow.Edges {
		draft.Edges = append(draft.Edges, WorkflowEdgeDraft{From: edge.From, To: edge.To})
	}
	return draft
}

// workflowFromDraft converts an editable draft into the runtime workflow that
// compilation and validation consume.
func workflowFromDraft(draft WorkflowDraft) *Workflow {
	workflow := &Workflow{
		ID:          draft.ID,
		Name:        draft.Name,
		Description: draft.Description,
		Version:     draft.Version,
		Defaults:    defaultsFromDraft(draft.Defaults),
		Nodes:       make([]Node, 0, len(draft.Nodes)),
		Edges:       make([]Edge, 0, len(draft.Edges)),
	}
	for _, node := range draft.Nodes {
		workflow.Nodes = append(workflow.Nodes, nodeFromDraft(node))
	}
	for _, edge := range draft.Edges {
		workflow.Edges = append(workflow.Edges, Edge{From: edge.From, To: edge.To})
	}
	return workflow
}

// defaultsDraftFromWorkflow converts workflow-wide settings.
func defaultsDraftFromWorkflow(defaults Defaults) WorkflowDefaultsDraft {
	return WorkflowDefaultsDraft{
		Harness:         defaults.Harness,
		Model:           modelDraftFromValue(defaults.Model),
		Interactive:     copyBool(defaults.Interactive),
		AllowInject:     copyBool(defaults.AllowInject),
		AskTimeout:      defaults.AskTimeout,
		ApprovalTimeout: defaults.ApprovalTimeout,
	}
}

// defaultsFromDraft converts editable workflow-wide settings.
func defaultsFromDraft(draft WorkflowDefaultsDraft) Defaults {
	return Defaults{
		Harness:         draft.Harness,
		Model:           modelValueFromDraft(draft.Model),
		Interactive:     copyBool(draft.Interactive),
		AllowInject:     copyBool(draft.AllowInject),
		AskTimeout:      draft.AskTimeout,
		ApprovalTimeout: draft.ApprovalTimeout,
	}
}

// nodeDraftFromWorkflow converts one node, trimming empty tool lists so the
// draft's "absent" and "empty" states agree.
func nodeDraftFromWorkflow(node Node) WorkflowNodeDraft {
	draft := WorkflowNodeDraft{
		ID:              node.ID,
		Title:           node.Title,
		Kind:            node.Kind,
		Harness:         node.Harness,
		Skill:           node.Skill,
		Skills:          append([]string(nil), node.Skills...),
		Prompt:          node.Prompt,
		Command:         node.Command,
		Workdir:         node.Workdir,
		Model:           modelDraftFromValue(node.Model),
		Interactive:     copyBool(node.Interactive),
		AllowInject:     copyBool(node.AllowInject),
		RequireApproval: node.RequireApproval,
		Timeout:         node.Timeout,
		AskTimeout:      node.AskTimeout,
		ApprovalTimeout: node.ApprovalTimeout,
		Retry:           node.Retry,
	}
	if len(node.Tools) > 0 {
		draft.Tools = append([]string(nil), node.Tools...)
	}
	if node.Postcondition != nil {
		postcondition := *node.Postcondition
		draft.Postcondition = &postcondition
	}
	return draft
}

// nodeFromDraft converts one editable node.
func nodeFromDraft(draft WorkflowNodeDraft) Node {
	node := Node{
		ID:              draft.ID,
		Title:           draft.Title,
		Kind:            draft.Kind,
		Harness:         draft.Harness,
		Skill:           draft.Skill,
		Skills:          append([]string(nil), draft.Skills...),
		Prompt:          draft.Prompt,
		Command:         draft.Command,
		Workdir:         draft.Workdir,
		Model:           modelValueFromDraft(draft.Model),
		Interactive:     copyBool(draft.Interactive),
		AllowInject:     copyBool(draft.AllowInject),
		RequireApproval: draft.RequireApproval,
		Timeout:         draft.Timeout,
		AskTimeout:      draft.AskTimeout,
		ApprovalTimeout: draft.ApprovalTimeout,
		Retry:           draft.Retry,
	}
	if len(draft.Tools) > 0 {
		node.Tools = append([]string(nil), draft.Tools...)
	}
	if draft.Postcondition != nil {
		postcondition := *draft.Postcondition
		node.Postcondition = &postcondition
	}
	return node
}

// modelDraftFromValue keeps an empty model absent rather than an empty object.
func modelDraftFromValue(value ModelValue) *ModelDraft {
	if value.ID == "" && value.Thinking == "" {
		return nil
	}
	return &ModelDraft{ID: value.ID, Thinking: value.Thinking}
}

// modelValueFromDraft converts an absent model to the zero value.
func modelValueFromDraft(draft *ModelDraft) ModelValue {
	if draft == nil {
		return ModelValue{}
	}
	return ModelValue{ID: draft.ID, Thinking: draft.Thinking}
}

// copyBool copies a tri-state boolean without collapsing nil to false.
func copyBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

// workflowManagedHeader is the fixed preamble of every file the editor writes.
const workflowManagedHeader = "# Managed by the SpecFlow workflow editor.\n" +
	"# Comments and formatting are not preserved on save.\n\n"

// workflowFileDocument is the canonical YAML shape. Unlike the runtime structs
// it omits empty optional values and spells model as a scalar when there is no
// reasoning effort.
type workflowFileDocument struct {
	ID          string                `yaml:"id"`
	Name        string                `yaml:"name,omitempty"`
	Description string                `yaml:"description,omitempty"`
	Version     int                   `yaml:"version,omitempty"`
	Defaults    *defaultsFileDocument `yaml:"defaults,omitempty"`
	Nodes       []nodeFileDocument    `yaml:"nodes"`
	Edges       []edgeFileDocument    `yaml:"edges,omitempty"`
}

// defaultsFileDocument is the canonical YAML shape of workflow defaults.
type defaultsFileDocument struct {
	Harness         string     `yaml:"harness,omitempty"`
	Model           *modelFile `yaml:"model,omitempty"`
	Interactive     *bool      `yaml:"interactive,omitempty"`
	AllowInject     *bool      `yaml:"allow_inject,omitempty"`
	AskTimeout      string     `yaml:"ask_timeout,omitempty"`
	ApprovalTimeout string     `yaml:"approval_timeout,omitempty"`
}

// nodeFileDocument is the canonical YAML shape of a node.
type nodeFileDocument struct {
	ID              string         `yaml:"id"`
	Title           string         `yaml:"title,omitempty"`
	Kind            NodeKind       `yaml:"kind"`
	Harness         string         `yaml:"harness,omitempty"`
	Skill           string         `yaml:"skill,omitempty"`
	Skills          []string       `yaml:"skills,omitempty"`
	Prompt          string         `yaml:"prompt,omitempty"`
	Command         string         `yaml:"command,omitempty"`
	Workdir         string         `yaml:"workdir,omitempty"`
	Model           *modelFile     `yaml:"model,omitempty"`
	Interactive     *bool          `yaml:"interactive,omitempty"`
	AllowInject     *bool          `yaml:"allow_inject,omitempty"`
	RequireApproval bool           `yaml:"require_approval,omitempty"`
	Timeout         string         `yaml:"timeout,omitempty"`
	AskTimeout      string         `yaml:"ask_timeout,omitempty"`
	ApprovalTimeout string         `yaml:"approval_timeout,omitempty"`
	Retry           int            `yaml:"retry,omitempty"`
	Tools           []string       `yaml:"tools,omitempty"`
	Postcondition   *Postcondition `yaml:"postcondition,omitempty"`
}

// edgeFileDocument is the canonical YAML shape of an ordering edge.
type edgeFileDocument struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// modelFile renders a model selection: a scalar when there is no reasoning
// effort, a mapping of id and thinking otherwise.
type modelFile struct {
	ID       string `yaml:"id,omitempty"`
	Thinking string `yaml:"thinking,omitempty"`
}

// MarshalYAML encodes the scalar form for a bare model identifier.
func (m modelFile) MarshalYAML() (any, error) {
	if m.Thinking == "" {
		return m.ID, nil
	}
	return struct {
		ID       string `yaml:"id,omitempty"`
		Thinking string `yaml:"thinking,omitempty"`
	}{ID: m.ID, Thinking: m.Thinking}, nil
}

// MarshalWorkflowYAML renders the canonical workflow file the editor writes:
// the managed-file header, snake_case keys, and no empty optional values.
func MarshalWorkflowYAML(draft WorkflowDraft) ([]byte, error) {
	document := workflowDocumentFromDraft(draft)
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode workflow %q: %w", draft.ID, err)
	}

	output := make([]byte, 0, len(workflowManagedHeader)+len(encoded))
	output = append(output, workflowManagedHeader...)
	output = append(output, encoded...)
	return output, nil
}

// workflowDocumentFromDraft builds the canonical YAML shape from a draft.
func workflowDocumentFromDraft(draft WorkflowDraft) workflowFileDocument {
	document := workflowFileDocument{
		ID:          draft.ID,
		Name:        draft.Name,
		Description: draft.Description,
		Version:     draft.Version,
		Defaults:    defaultsDocumentFromDraft(draft.Defaults),
		Nodes:       make([]nodeFileDocument, 0, len(draft.Nodes)),
	}
	for _, node := range draft.Nodes {
		document.Nodes = append(document.Nodes, nodeDocumentFromDraft(node))
	}
	for _, edge := range draft.Edges {
		document.Edges = append(document.Edges, edgeFileDocument{From: edge.From, To: edge.To})
	}
	return document
}

// defaultsDocumentFromDraft omits defaults entirely when they are empty.
func defaultsDocumentFromDraft(draft WorkflowDefaultsDraft) *defaultsFileDocument {
	document := defaultsFileDocument{
		Harness:         draft.Harness,
		Model:           modelFileFromDraft(draft.Model),
		Interactive:     copyBool(draft.Interactive),
		AllowInject:     copyBool(draft.AllowInject),
		AskTimeout:      draft.AskTimeout,
		ApprovalTimeout: draft.ApprovalTimeout,
	}
	if document == (defaultsFileDocument{}) {
		return nil
	}
	return &document
}

// nodeDocumentFromDraft builds the canonical YAML shape of one node.
func nodeDocumentFromDraft(draft WorkflowNodeDraft) nodeFileDocument {
	document := nodeFileDocument{
		ID:              draft.ID,
		Title:           draft.Title,
		Kind:            draft.Kind,
		Harness:         draft.Harness,
		Prompt:          draft.Prompt,
		Command:         draft.Command,
		Workdir:         draft.Workdir,
		Model:           modelFileFromDraft(draft.Model),
		Interactive:     copyBool(draft.Interactive),
		AllowInject:     copyBool(draft.AllowInject),
		RequireApproval: draft.RequireApproval,
		Timeout:         draft.Timeout,
		AskTimeout:      draft.AskTimeout,
		ApprovalTimeout: draft.ApprovalTimeout,
		Retry:           draft.Retry,
	}
	if len(draft.Skills) > 0 {
		document.Skills = append([]string(nil), draft.Skills...)
	} else {
		document.Skill = draft.Skill
	}
	if len(draft.Tools) > 0 {
		document.Tools = append([]string(nil), draft.Tools...)
	}
	if draft.Postcondition != nil {
		postcondition := *draft.Postcondition
		document.Postcondition = &postcondition
	}
	return document
}

// modelFileFromDraft keeps an empty model out of the canonical file.
func modelFileFromDraft(draft *ModelDraft) *modelFile {
	if draft == nil || (draft.ID == "" && draft.Thinking == "") {
		return nil
	}
	return &modelFile{ID: draft.ID, Thinking: draft.Thinking}
}
