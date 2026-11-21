package orchestrator

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// NodeKind is what executes a node.
type NodeKind string

const (
	// NodeKindAgent runs through a harness adapter.
	NodeKindAgent NodeKind = "agent"
	// NodeKindCommand runs a shell command in the node's working directory.
	NodeKindCommand NodeKind = "command"
)

// Workdir values a node may declare.
const (
	WorkdirRun     = "run"
	WorkdirProject = "project"
)

// ModelValue is a model selection in the workflow file. YAML may spell it as a
// bare identifier (`model: default`) or as a mapping with reasoning effort.
type ModelValue struct {
	ID       string
	Thinking string
}

// UnmarshalYAML accepts either a scalar model identifier or a mapping.
func (m *ModelValue) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var id string
		if err := value.Decode(&id); err != nil {
			return err
		}
		m.ID = id
		return nil
	}

	var raw struct {
		ID       string `yaml:"id"`
		Thinking string `yaml:"thinking"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	m.ID = raw.ID
	m.Thinking = raw.Thinking
	return nil
}

// EffectiveSkills returns the node's skill bindings, preferring the plural
// form while keeping the original singular field readable.
func (n Node) EffectiveSkills() []string {
	if len(n.Skills) > 0 {
		return n.Skills
	}
	if strings.TrimSpace(n.Skill) != "" {
		return []string{n.Skill}
	}
	return nil
}

// Postcondition is how a node proves it did its job. It is evaluated against
// the repository, not against the harness's own account of what it did.
type Postcondition struct {
	Type        string `yaml:"type" json:"type"`
	Path        string `yaml:"path,omitempty" json:"path,omitempty"`
	Change      string `yaml:"change,omitempty" json:"change,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Requirement reports that a postcondition did not hold.
func (p *Postcondition) Requirement() string {
	if p == nil {
		return ""
	}
	if p.Description != "" {
		return p.Description
	}
	return p.Type
}

// Defaults are workflow-wide node settings.
type Defaults struct {
	Harness         string     `yaml:"harness"`
	Model           ModelValue `yaml:"model"`
	Interactive     *bool      `yaml:"interactive"`
	AllowInject     *bool      `yaml:"allow_inject"`
	AskTimeout      string     `yaml:"ask_timeout"`
	ApprovalTimeout string     `yaml:"approval_timeout"`
}

// Node is one block in a workflow. Its name carries no behavior: everything
// that makes it a phase is a field here.
type Node struct {
	ID              string         `yaml:"id"`
	Title           string         `yaml:"title"`
	Kind            NodeKind       `yaml:"kind"`
	Harness         string         `yaml:"harness"`
	Skill           string         `yaml:"skill,omitempty"`
	Skills          []string       `yaml:"skills,omitempty"`
	Prompt          string         `yaml:"prompt"`
	Command         string         `yaml:"command"`
	Workdir         string         `yaml:"workdir"`
	Model           ModelValue     `yaml:"model"`
	Interactive     *bool          `yaml:"interactive"`
	AllowInject     *bool          `yaml:"allow_inject"`
	RequireApproval bool           `yaml:"require_approval"`
	Timeout         string         `yaml:"timeout"`
	AskTimeout      string         `yaml:"ask_timeout"`
	ApprovalTimeout string         `yaml:"approval_timeout"`
	Retry           int            `yaml:"retry"`
	Tools           []string       `yaml:"tools"`
	Postcondition   *Postcondition `yaml:"postcondition"`
	// Routes maps an exit code to the route label the node completes under.
	// Only a command node has exit codes, so only a command node may route.
	Routes map[int]string `yaml:"routes,omitempty"`
}

// Edge is a directed dependency between nodes. It carries ordering and, when
// its source declares routes, the route label that selects it.
type Edge struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	// When names the route label of the source node that takes this edge.
	When string `yaml:"when,omitempty"`
}

// RouteCodes returns the node's declared exit codes in ascending order, so a
// caller that reports per-code defects does so deterministically.
func (n *Node) RouteCodes() []int {
	codes := make([]int, 0, len(n.Routes))
	for code := range n.Routes {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	return codes
}

// Workflow is a graph of nodes with ordering edges.
type Workflow struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Version     int      `yaml:"version"`
	Defaults    Defaults `yaml:"defaults"`
	Nodes       []Node   `yaml:"nodes"`
	Edges       []Edge   `yaml:"edges"`
}

// LoadWorkflow reads and parses a workflow file.
func LoadWorkflow(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow %s: %w", path, err)
	}
	return ParseWorkflow(data, path)
}

// ParseWorkflow parses workflow source. The source label is used as the
// workflow identity when the file declares none, so compiling a retained copy
// does not change that identity.
func ParseWorkflow(data []byte, source string) (*Workflow, error) {
	var workflow Workflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("parse workflow %s: %w", source, err)
	}
	if workflow.ID == "" {
		workflow.ID = source
	}
	return &workflow, nil
}

// Node returns the node with an identifier.
func (w *Workflow) Node(id string) *Node {
	return findNode(w.Nodes, id, func(node Node) string { return node.ID })
}

// findNode returns the first item whose identifier matches.
func findNode[T any](items []T, id string, identifier func(T) string) *T {
	for index := range items {
		if identifier(items[index]) == id {
			return &items[index]
		}
	}
	return nil
}

// EffectiveHarness is the harness a node runs on, falling back to the workflow
// default.
func (w *Workflow) EffectiveHarness(node *Node) string {
	if node.Harness != "" {
		return node.Harness
	}
	return w.Defaults.Harness
}

// EffectiveInteractive reports whether a node may ask the user questions.
func (w *Workflow) EffectiveInteractive(node *Node) bool {
	if node.Interactive != nil {
		return *node.Interactive
	}
	if w.Defaults.Interactive != nil {
		return *w.Defaults.Interactive
	}
	return false
}

// EffectiveInject reports whether a node accepts mid-run clarifications.
func (w *Workflow) EffectiveInject(node *Node) bool {
	if node.AllowInject != nil {
		return *node.AllowInject
	}
	if w.Defaults.AllowInject != nil {
		return *w.Defaults.AllowInject
	}
	return false
}

// EffectiveAskTimeout resolves the configured question deadline: node value,
// else workflow default, else zero (no configured deadline).
func (w *Workflow) EffectiveAskTimeout(node *Node) (time.Duration, error) {
	if node.AskTimeout != "" {
		parsed, err := time.ParseDuration(node.AskTimeout)
		if err != nil {
			return 0, fmt.Errorf("node %q has invalid ask_timeout %q", node.ID, node.AskTimeout)
		}
		return parsed, nil
	}
	if w.Defaults.AskTimeout != "" {
		parsed, err := time.ParseDuration(w.Defaults.AskTimeout)
		if err != nil {
			return 0, fmt.Errorf("workflow defaults have invalid ask_timeout %q", w.Defaults.AskTimeout)
		}
		return parsed, nil
	}
	return 0, nil
}

// EffectiveApprovalTimeout resolves the configured approval deadline: node
// value, else workflow default, else zero (no configured deadline).
func (w *Workflow) EffectiveApprovalTimeout(node *Node) (time.Duration, error) {
	if node.ApprovalTimeout != "" {
		parsed, err := time.ParseDuration(node.ApprovalTimeout)
		if err != nil {
			return 0, fmt.Errorf("node %q has invalid approval_timeout %q", node.ID, node.ApprovalTimeout)
		}
		return parsed, nil
	}
	if w.Defaults.ApprovalTimeout != "" {
		parsed, err := time.ParseDuration(w.Defaults.ApprovalTimeout)
		if err != nil {
			return 0, fmt.Errorf("workflow defaults have invalid approval_timeout %q", w.Defaults.ApprovalTimeout)
		}
		return parsed, nil
	}
	return 0, nil
}

// Timeout returns the node's timeout, or a kind-dependent default.
func (n *Node) TimeoutDuration() time.Duration {
	if n.Timeout != "" {
		if parsed, err := time.ParseDuration(n.Timeout); err == nil {
			return parsed
		}
	}
	if n.Kind == NodeKindCommand {
		return 10 * time.Minute
	}
	return 30 * time.Minute
}

// Order returns node identifiers in a stable topological order: a node starts
// only after its predecessors, and ties keep declaration order.
func (w *Workflow) Order() ([]string, error) {
	indegree := make(map[string]int, len(w.Nodes))
	outgoing := make(map[string][]string, len(w.Nodes))
	for _, node := range w.Nodes {
		indegree[node.ID] = 0
	}
	for _, edge := range w.Edges {
		if _, ok := indegree[edge.From]; !ok {
			return nil, fmt.Errorf("edge references unknown node %q", edge.From)
		}
		if _, ok := indegree[edge.To]; !ok {
			return nil, fmt.Errorf("edge references unknown node %q", edge.To)
		}
		outgoing[edge.From] = append(outgoing[edge.From], edge.To)
		indegree[edge.To]++
	}

	remaining := make([]string, 0, len(w.Nodes))
	for _, node := range w.Nodes {
		remaining = append(remaining, node.ID)
	}

	order := make([]string, 0, len(remaining))
	for len(remaining) > 0 {
		index := -1
		for i, id := range remaining {
			if indegree[id] == 0 {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, fmt.Errorf("workflow contains a cycle among %v", remaining)
		}

		id := remaining[index]
		remaining = append(remaining[:index], remaining[index+1:]...)
		order = append(order, id)

		for _, next := range outgoing[id] {
			indegree[next]--
		}
	}

	return order, nil
}
