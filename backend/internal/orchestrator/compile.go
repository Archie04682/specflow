package orchestrator

import (
	"fmt"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/openspec"
)

// NormalizeContext is what normalization needs to know about the project.
// Skill roots are the only project state a plan depends on.
type NormalizeContext struct {
	SkillRoots []string
}

// NormalizeWorkflow resolves every node's effective values into an executable
// plan. It reports every node that cannot be resolved in one error, the same
// way ValidateWorkflow reports every structural defect at once.
func NormalizeWorkflow(workflow *Workflow, context NormalizeContext) (*WorkflowPlan, error) {
	plan := &WorkflowPlan{
		ID:          workflow.ID,
		Name:        workflow.Name,
		Description: workflow.Description,
		Version:     workflow.Version,
		Nodes:       make([]PlanNode, 0, len(workflow.Nodes)),
	}

	var problems []string
	for index := range workflow.Nodes {
		node, err := normalizeNode(workflow, &workflow.Nodes[index], context)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		plan.Nodes = append(plan.Nodes, node)
	}
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}

	order, err := workflow.Order()
	if err != nil {
		return nil, fmt.Errorf("order workflow %s: %w", workflow.ID, err)
	}
	plan.Order = order
	return plan, nil
}

func normalizeNode(workflow *Workflow, node *Node, context NormalizeContext) (PlanNode, error) {
	askTimeout, err := workflow.EffectiveAskTimeout(node)
	if err != nil {
		return PlanNode{}, err
	}
	approvalTimeout, err := workflow.EffectiveApprovalTimeout(node)
	if err != nil {
		return PlanNode{}, err
	}

	planNode := PlanNode{
		ID:              node.ID,
		Title:           node.Title,
		Kind:            node.Kind,
		Interactive:     workflow.EffectiveInteractive(node),
		AllowInject:     workflow.EffectiveInject(node),
		Workdir:         node.Workdir,
		Timeout:         Duration(node.TimeoutDuration()),
		AskTimeout:      Duration(askTimeout),
		ApprovalTimeout: Duration(approvalTimeout),
		Retry:           node.Retry,
		Tools:           append([]string(nil), node.Tools...),
		Prompt:          node.Prompt,
		Command:         node.Command,
		RequireApproval: node.RequireApproval,
	}
	if planNode.Workdir == "" {
		planNode.Workdir = WorkdirRun
	}
	if planNode.Workdir != WorkdirRun && planNode.Workdir != WorkdirProject {
		return PlanNode{}, fmt.Errorf("node %q has unknown workdir %q", node.ID, node.Workdir)
	}

	if node.Kind == NodeKindAgent {
		planNode.Harness = workflow.EffectiveHarness(node)
		model := ResolveModel(node, workflow.Defaults)
		if model.ID != "" || model.Thinking != "" {
			planNode.Model = &model
		}
	}

	if node.Postcondition != nil {
		postcondition := *node.Postcondition
		planNode.Postcondition = &postcondition
	}

	names := node.EffectiveSkills()
	resolved := make([]harness.Skill, 0, len(names))
	for _, name := range names {
		path, err := openspec.ResolveSkill(name, context.SkillRoots)
		if err != nil {
			return PlanNode{}, fmt.Errorf("node %q: %w", node.ID, err)
		}
		resolved = append(resolved, harness.Skill{Name: name, Paths: []string{path}})
	}
	if len(resolved) > 0 {
		planNode.Skills = resolved
		planNode.Skill = &resolved[0]
	}

	return planNode, nil
}

// CompileWorkflow parses, validates, and normalizes one workflow source.
// Structural defects short-circuit normalization; resolution defects are
// reported together. Model identifiers are never checked: the adapter owns
// that failure.
func CompileWorkflow(data []byte, source string, context NormalizeContext) (*WorkflowPlan, error) {
	workflow, err := ParseWorkflow(data, source)
	if err != nil {
		return nil, err
	}
	if err := ValidateWorkflow(workflow); err != nil {
		return nil, err
	}

	plan, err := NormalizeWorkflow(workflow, context)
	if err != nil {
		return nil, err
	}
	plan.Source = source
	return plan, nil
}
