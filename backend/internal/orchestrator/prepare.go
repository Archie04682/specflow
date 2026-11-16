package orchestrator

import (
	"fmt"
	"regexp"
	"strings"

	"example.com/specflow/backend/internal/harness"
)

var promptPlaceholder = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

// RenderPrompt substitutes the closed set of run-context values. Anything else
// is an error: a typo must fail before a node starts rather than reach a model
// as literal braces.
func RenderPrompt(template string, context RunContext) (string, error) {
	values := context.Values()

	var unsupported []string
	rendered := promptPlaceholder.ReplaceAllStringFunc(template, func(match string) string {
		key := strings.TrimSpace(match[2 : len(match)-2])
		value, ok := values[key]
		if !ok {
			unsupported = append(unsupported, key)
			return match
		}
		return value
	})

	if len(unsupported) > 0 {
		return "", fmt.Errorf("unsupported prompt placeholder %q", unsupported[0])
	}
	return rendered, nil
}

// ResolveModel applies the node-over-workflow-over-harness chain. An empty
// value falls through; the literal "default" means "stop here and let the
// harness choose", which is different from leaving the field empty.
func ResolveModel(node *Node, defaults Defaults) harness.Model {
	return harness.Model{
		ID:       resolveModelField(node.Model.ID, defaults.Model.ID),
		Thinking: resolveModelField(node.Model.Thinking, defaults.Model.Thinking),
	}
}

func resolveModelField(nodeValue, defaultValue string) string {
	nodeValue = strings.TrimSpace(nodeValue)
	if nodeValue != "" {
		if nodeValue == "default" {
			return ""
		}
		return nodeValue
	}

	defaultValue = strings.TrimSpace(defaultValue)
	if defaultValue == "default" {
		return ""
	}
	return defaultValue
}

// ResolveWorkdir returns the directory a node executes in.
func ResolveWorkdir(node *PlanNode, worktree, projectRoot string) (string, error) {
	switch node.Workdir {
	case "", WorkdirRun:
		return worktree, nil
	case WorkdirProject:
		return projectRoot, nil
	default:
		return "", fmt.Errorf("node %q has unknown workdir %q", node.ID, node.Workdir)
	}
}

// Preflight checks a node's declared requirements against the adapter that
// will run it. It runs before the node starts so an unmet requirement costs a
// message instead of a run.
func Preflight(node *PlanNode, capabilities harness.Capabilities) error {
	modelID := ""
	if node.Model != nil {
		modelID = node.Model.ID
	}
	skillName := ""
	if len(node.Skills) > 0 {
		skillName = node.Skills[0].Name
	} else if node.Skill != nil {
		skillName = node.Skill.Name
	}

	needs := harness.Needs{
		NodeID:      node.ID,
		Interactive: node.Interactive,
		Inject:      node.AllowInject,
		Skill:       skillName,
		Model:       modelID,
		Tools:       node.Tools,
	}

	if failure := harness.Validate(needs, capabilities); failure != nil {
		return failure
	}
	return nil
}
