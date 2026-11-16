package orchestrator

import (
	"fmt"
	"strings"
	"time"
)

var postconditionTypes = map[string]bool{
	"none":           true,
	"file_exists":    true,
	"new_change":     true,
	"tasks_complete": true,
	"exit_zero":      true,
}

// ValidationError reports every defect found in one response, so a user fixes
// the workflow in one pass instead of one error at a time.
type ValidationError struct {
	Problems []string
}

// Error implements error.
func (e *ValidationError) Error() string {
	return "workflow is invalid: " + strings.Join(e.Problems, "; ")
}

// ValidateWorkflow checks a workflow before a run is created.
func ValidateWorkflow(workflow *Workflow) error {
	var problems []string

	if len(workflow.Nodes) == 0 {
		problems = append(problems, "workflow has no nodes")
	}
	problems = append(problems, validateDefaults(workflow.Defaults)...)

	seen := make(map[string]bool, len(workflow.Nodes))
	for _, node := range workflow.Nodes {
		problems = append(problems, validateNode(workflow, node, seen)...)
	}

	// The cycle walk needs a connected graph: reporting a dangling edge is
	// more useful than a second error that only repeats it.
	if validateEdges(workflow, seen, &problems) {
		if _, err := workflow.Order(); err != nil {
			problems = append(problems, err.Error())
		}
	}

	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// validateNode checks one node's kind-specific and common fields, recording its
// identifier in seen so duplicates and edges can be checked in the same pass.
func validateNode(workflow *Workflow, node Node, seen map[string]bool) []string {
	var problems []string

	if node.ID == "" {
		return append(problems, "a node has an empty id")
	}
	if seen[node.ID] {
		problems = append(problems, fmt.Sprintf("duplicate node id %q", node.ID))
	}
	seen[node.ID] = true

	switch node.Kind {
	case NodeKindAgent:
		if workflow.EffectiveHarness(&node) == "" {
			problems = append(problems, fmt.Sprintf("agent node %q names no harness", node.ID))
		}
	case NodeKindCommand:
		if strings.TrimSpace(node.Command) == "" {
			problems = append(problems, fmt.Sprintf("command node %q declares no command", node.ID))
		}
	default:
		problems = append(problems, fmt.Sprintf("node %q has unknown kind %q", node.ID, node.Kind))
	}

	if node.Workdir != "" && node.Workdir != WorkdirRun && node.Workdir != WorkdirProject {
		problems = append(problems, fmt.Sprintf("node %q has unknown workdir %q", node.ID, node.Workdir))
	}
	if node.Skill != "" && len(node.Skills) > 0 {
		problems = append(problems, fmt.Sprintf("node %q sets both skill and skills", node.ID))
	}
	problems = append(problems, validateNodeTimeouts(node)...)
	if node.Postcondition != nil {
		problems = append(problems, validatePostcondition(&node)...)
	}
	return problems
}

// validateNodeTimeouts checks the node's execution and interaction deadlines.
func validateNodeTimeouts(node Node) []string {
	var problems []string
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "timeout", value: node.Timeout},
		{name: "ask_timeout", value: node.AskTimeout},
		{name: "approval_timeout", value: node.ApprovalTimeout},
	} {
		if field.value == "" {
			continue
		}
		if _, err := time.ParseDuration(field.value); err != nil {
			problems = append(problems, fmt.Sprintf("node %q has invalid %s %q", node.ID, field.name, field.value))
		}
	}
	return problems
}

// validateDefaults checks the workflow-wide interaction deadlines. They are
// optional; a present value must parse.
func validateDefaults(defaults Defaults) []string {
	var problems []string
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "ask_timeout", value: defaults.AskTimeout},
		{name: "approval_timeout", value: defaults.ApprovalTimeout},
	} {
		if field.value == "" {
			continue
		}
		if _, err := time.ParseDuration(field.value); err != nil {
			problems = append(problems, fmt.Sprintf("workflow defaults have invalid %s %q", field.name, field.value))
		}
	}
	return problems
}

// validateEdges reports edge endpoints that name no node, and returns whether
// every endpoint resolved.
func validateEdges(workflow *Workflow, seen map[string]bool, problems *[]string) bool {
	valid := true
	for _, edge := range workflow.Edges {
		if !seen[edge.From] {
			*problems = append(*problems, fmt.Sprintf("edge references unknown node %q", edge.From))
			valid = false
		}
		if !seen[edge.To] {
			*problems = append(*problems, fmt.Sprintf("edge references unknown node %q", edge.To))
			valid = false
		}
	}
	return valid
}

func validatePostcondition(node *Node) []string {
	postcondition := node.Postcondition
	var problems []string

	if !postconditionTypes[postcondition.Type] {
		problems = append(problems, fmt.Sprintf("node %q has unknown postcondition type %q", node.ID, postcondition.Type))
	}
	if postcondition.Type == "file_exists" && postcondition.Path == "" {
		problems = append(problems, fmt.Sprintf("node %q has a file_exists postcondition without a path", node.ID))
	}
	if postcondition.Type == "exit_zero" && node.Kind != NodeKindCommand {
		problems = append(problems, fmt.Sprintf("node %q declares exit_zero but is not a command node", node.ID))
	}
	return problems
}
