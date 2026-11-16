package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"example.com/specflow/backend/internal/workspace"
)

// GraphNode is one node of a workflow as the read surface presents it: the
// authored identity plus the flags needed before execution starts. Interactive
// and injection are effective values (node override, else workflow default);
// approval is the node's own declaration.
type GraphNode struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Kind            NodeKind `json:"kind"`
	Interactive     bool     `json:"interactive"`
	AllowInject     bool     `json:"allowInject"`
	RequireApproval bool     `json:"requireApproval"`
}

// GraphEdge is one authored ordering edge.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// WorkflowSummary is one workflow the project can start a run from. It is the
// structural view: nothing here is compiled, resolved, or probed, so a file
// that binds a missing skill is still valid and a broken file is still listed
// with its problems.
type WorkflowSummary struct {
	Reference   string      `json:"reference"`
	Path        string      `json:"path"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Version     int         `json:"version"`
	Valid       bool        `json:"valid"`
	Problems    []string    `json:"problems"`
	Nodes       []GraphNode `json:"nodes"`
	Edges       []GraphEdge `json:"edges"`
}

// workflowFile pairs a reference with the path that wins for it, so sorting is
// deterministic across the two directories.
type workflowFile struct {
	reference string
	path      string
}

// ReadWorkflowCatalog lists the project's workflows without compiling them.
// The project's own .specflow/workflows directory takes precedence over the
// repository workflows directory for a shared reference. Entries are ordered
// by reference; each entry is validated structurally only.
func ReadWorkflowCatalog(ctx context.Context, projectRoot string) ([]WorkflowSummary, error) {
	project, err := workspace.Open(ctx, projectRoot)
	if err != nil {
		return nil, err
	}

	files, err := workflowFiles(project.Root)
	if err != nil {
		return nil, err
	}

	catalog := make([]WorkflowSummary, 0, len(files))
	for _, file := range files {
		catalog = append(catalog, readWorkflowSummary(file.reference, file.path))
	}
	return catalog, nil
}

// workflowFiles enumerates the .yaml files directly under the project's
// workflow directories, in resolution precedence, and returns them sorted by
// reference then path. A missing directory contributes nothing.
func workflowFiles(projectRoot string) ([]workflowFile, error) {
	directories := []string{
		filepath.Join(projectRoot, ".specflow", "workflows"),
		filepath.Join(projectRoot, "workflows"),
	}

	seen := make(map[string]bool)
	files := make([]workflowFile, 0)
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read workflow directory %s: %w", directory, err)
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			reference := strings.TrimSuffix(entry.Name(), ".yaml")
			if seen[reference] {
				continue
			}
			seen[reference] = true
			files = append(files, workflowFile{
				reference: reference,
				path:      filepath.Join(directory, entry.Name()),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].reference != files[j].reference {
			return files[i].reference < files[j].reference
		}
		return files[i].path < files[j].path
	})
	return files, nil
}

// readWorkflowSummary parses and structurally validates one file. A read or
// parse failure is reported on the entry instead of failing the catalog; the
// nodes and edges are empty (not nil) so the JSON is an empty array.
func readWorkflowSummary(reference, path string) WorkflowSummary {
	summary := WorkflowSummary{
		Reference: reference,
		Path:      path,
		Name:      reference,
		Problems:  []string{},
		Nodes:     []GraphNode{},
		Edges:     []GraphEdge{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		summary.Problems = append(summary.Problems, fmt.Errorf("read workflow %s: %w", path, err).Error())
		return summary
	}

	workflow, err := ParseWorkflow(data, path)
	if err != nil {
		summary.Problems = append(summary.Problems, err.Error())
		return summary
	}

	summary.Name = workflow.Name
	if summary.Name == "" {
		summary.Name = reference
	}
	summary.Description = workflow.Description
	summary.Version = workflow.Version
	summary.Nodes = graphNodes(workflow)
	summary.Edges = graphEdges(workflow)

	if err := ValidateWorkflow(workflow); err != nil {
		var validation *ValidationError
		if errors.As(err, &validation) {
			summary.Problems = append(summary.Problems, validation.Problems...)
		} else {
			summary.Problems = append(summary.Problems, err.Error())
		}
		return summary
	}

	summary.Valid = true
	return summary
}

// graphNodes projects nodes in declaration order with their effective flags.
func graphNodes(workflow *Workflow) []GraphNode {
	nodes := make([]GraphNode, 0, len(workflow.Nodes))
	for index := range workflow.Nodes {
		node := &workflow.Nodes[index]
		nodes = append(nodes, GraphNode{
			ID:              node.ID,
			Title:           node.Title,
			Kind:            node.Kind,
			Interactive:     workflow.EffectiveInteractive(node),
			AllowInject:     workflow.EffectiveInject(node),
			RequireApproval: node.RequireApproval,
		})
	}
	return nodes
}

// graphEdges projects the authored edges in declaration order.
func graphEdges(workflow *Workflow) []GraphEdge {
	edges := make([]GraphEdge, 0, len(workflow.Edges))
	for _, edge := range workflow.Edges {
		edges = append(edges, GraphEdge{From: edge.From, To: edge.To})
	}
	return edges
}
