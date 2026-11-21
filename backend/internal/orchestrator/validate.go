package orchestrator

import (
	"fmt"
	"sort"
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
		problems = append(problems, validateRouting(workflow)...)
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

// validateRouting checks the routing declaration: which nodes may route, which
// labels they declare, and which edges select them. Routing is a total
// function, so every declared label is used by exactly one outgoing edge and
// every outgoing edge of a routing node names a declared label.
func validateRouting(workflow *Workflow) []string {
	var problems []string

	// labels holds each routing node's declared labels; a node with no routes
	// is absent, so "declares routes" and "its label set" stay distinguishable.
	labels := make(map[string]map[string]bool, len(workflow.Nodes))
	for index := range workflow.Nodes {
		node := &workflow.Nodes[index]
		if len(node.Routes) == 0 {
			continue
		}
		if node.Kind != NodeKindCommand {
			problems = append(problems, fmt.Sprintf("node %q declares routes but is not a command node", node.ID))
		}
		if node.Postcondition != nil {
			problems = append(problems, fmt.Sprintf("node %q declares both routes and a postcondition", node.ID))
		}

		set := make(map[string]bool, len(node.Routes))
		for _, code := range node.RouteCodes() {
			label := strings.TrimSpace(node.Routes[code])
			if label == "" {
				problems = append(problems, fmt.Sprintf("node %q declares an empty route label for exit code %d", node.ID, code))
				continue
			}
			set[label] = true
		}
		labels[node.ID] = set
	}
	if len(labels) == 0 {
		return problems
	}

	used := make(map[string]int)
	for _, edge := range workflow.Edges {
		set, routing := labels[edge.From]
		if !routing {
			if edge.When != "" {
				problems = append(problems, fmt.Sprintf(
					"edge %q -> %q declares when %q but node %q declares no routes", edge.From, edge.To, edge.When, edge.From))
			}
			continue
		}
		if edge.When == "" {
			problems = append(problems, fmt.Sprintf(
				"edge %q -> %q declares no when, but node %q declares routes", edge.From, edge.To, edge.From))
			continue
		}
		if !set[edge.When] {
			problems = append(problems, fmt.Sprintf(
				"edge %q -> %q declares when %q which node %q does not route", edge.From, edge.To, edge.When, edge.From))
			continue
		}
		used[edge.From+"\x00"+edge.When]++
		if used[edge.From+"\x00"+edge.When] == 2 {
			problems = append(problems, fmt.Sprintf(
				"node %q routes %q on more than one outgoing edge", edge.From, edge.When))
		}
	}

	// Walk the declaration order, not the label map, so the same workflow
	// always reports its unused labels in the same order.
	for index := range workflow.Nodes {
		node := &workflow.Nodes[index]
		set, routing := labels[node.ID]
		if !routing {
			continue
		}
		for _, label := range sortedLabels(set) {
			if used[node.ID+"\x00"+label] == 0 {
				problems = append(problems, fmt.Sprintf(
					"node %q declares route %q but no outgoing edge uses it", node.ID, label))
			}
		}
	}
	return problems
}

// sortedLabels returns a label set's members in ascending order.
func sortedLabels(set map[string]bool) []string {
	labels := make([]string, 0, len(set))
	for label := range set {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
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
