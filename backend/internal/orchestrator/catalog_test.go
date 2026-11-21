package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const catalogWorkflow = `
id: alpha
name: Alpha
description: the first workflow
version: 2
defaults:
  harness: fake
  interactive: true
  allow_inject: true
nodes:
  - {id: explore, title: Explore, kind: agent, prompt: look}
  - {id: apply, title: Apply, kind: agent, prompt: work, interactive: false, require_approval: true}
edges:
  - {from: explore, to: apply}
`

func catalogEntry(t *testing.T, entries []WorkflowSummary, reference string) WorkflowSummary {
	t.Helper()

	for _, entry := range entries {
		if entry.Reference == reference {
			return entry
		}
	}
	t.Fatalf("catalog has no entry %q: %+v", reference, entries)
	return WorkflowSummary{}
}

func catalogReferences(entries []WorkflowSummary) []string {
	references := make([]string, 0, len(entries))
	for _, entry := range entries {
		references = append(references, entry.Reference)
	}
	return references
}

func TestReadWorkflowCatalogListsBothDirectories(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "zeta.yaml"), `
id: zeta
defaults: {harness: fake}
nodes:
  - {id: only, kind: agent}
`)
	writeTestFile(t, filepath.Join(project, "workflows", "alpha.yaml"), catalogWorkflow)
	// Neither a non-YAML file nor a nested workflow is part of the catalog.
	writeTestFile(t, filepath.Join(project, "workflows", "notes.txt"), "not a workflow")
	writeTestFile(t, filepath.Join(project, "workflows", "nested", "deep.yaml"), "id: deep\n")

	catalog, err := ReadWorkflowCatalog(context.Background(), project)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	if got, want := catalogReferences(catalog), []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("references = %v, want %v", got, want)
	}

	alpha := catalogEntry(t, catalog, "alpha")
	if want := filepath.Join(project, "workflows", "alpha.yaml"); alpha.Path != want {
		t.Fatalf("alpha path = %q, want %q", alpha.Path, want)
	}
	if alpha.Name != "Alpha" || alpha.Description != "the first workflow" || alpha.Version != 2 {
		t.Fatalf("alpha metadata = %+v", alpha)
	}
	if !alpha.Valid {
		t.Fatalf("alpha is invalid: %v", alpha.Problems)
	}
	if len(alpha.Problems) != 0 {
		t.Fatalf("alpha problems = %v", alpha.Problems)
	}

	wantNodes := []GraphNode{
		{ID: "explore", Title: "Explore", Kind: NodeKindAgent, Interactive: true, AllowInject: true},
		{ID: "apply", Title: "Apply", Kind: NodeKindAgent, Interactive: false, AllowInject: true, RequireApproval: true},
	}
	if !reflect.DeepEqual(alpha.Nodes, wantNodes) {
		t.Fatalf("alpha nodes = %+v, want %+v", alpha.Nodes, wantNodes)
	}
	if want := []GraphEdge{{From: "explore", To: "apply"}}; !reflect.DeepEqual(alpha.Edges, want) {
		t.Fatalf("alpha edges = %+v, want %+v", alpha.Edges, want)
	}

	zeta := catalogEntry(t, catalog, "zeta")
	if zeta.Name != "zeta" {
		t.Fatalf("zeta name = %q, want the reference fallback", zeta.Name)
	}
	if !zeta.Valid {
		t.Fatalf("zeta is invalid: %v", zeta.Problems)
	}
	if len(zeta.Edges) != 0 {
		t.Fatalf("zeta edges = %+v, want none", zeta.Edges)
	}
}

func TestReadWorkflowCatalogPrefersTheProjectCopy(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "shared.yaml"), `
id: project-copy
name: Project copy
defaults: {harness: fake}
nodes:
  - {id: project, kind: agent}
`)
	writeTestFile(t, filepath.Join(project, "workflows", "shared.yaml"), `
id: repo-copy
name: Repository copy
defaults: {harness: fake}
nodes:
  - {id: repo, kind: agent}
`)

	catalog, err := ReadWorkflowCatalog(context.Background(), project)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("catalog = %+v, want one entry", catalog)
	}

	entry := catalog[0]
	if want := filepath.Join(project, ".specflow", "workflows", "shared.yaml"); entry.Path != want {
		t.Fatalf("path = %q, want %q", entry.Path, want)
	}
	if entry.Name != "Project copy" {
		t.Fatalf("name = %q, want the project copy", entry.Name)
	}
	if len(entry.Nodes) != 1 || entry.Nodes[0].ID != "project" {
		t.Fatalf("nodes = %+v, want the project copy", entry.Nodes)
	}
}

func TestReadWorkflowCatalogWithoutWorkflowDirectories(t *testing.T) {
	project := newRepoProject(t)

	catalog, err := ReadWorkflowCatalog(context.Background(), project)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if catalog == nil {
		t.Fatal("catalog is nil, want an empty slice")
	}
	if len(catalog) != 0 {
		t.Fatalf("catalog = %+v, want empty", catalog)
	}
}

func TestReadWorkflowCatalogListsBrokenFiles(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "broken.yaml"), "nodes: [\n  - {\n")
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "good.yaml"), `
id: good
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent}
`)

	catalog, err := ReadWorkflowCatalog(context.Background(), project)
	if err != nil {
		t.Fatalf("a broken file failed the catalog: %v", err)
	}
	if got, want := catalogReferences(catalog), []string{"broken", "good"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("references = %v, want %v", got, want)
	}

	broken := catalogEntry(t, catalog, "broken")
	if broken.Valid {
		t.Fatal("unparseable workflow reported as valid")
	}
	if len(broken.Problems) == 0 {
		t.Fatal("unparseable workflow reported no problems")
	}
	if broken.Name != "broken" {
		t.Fatalf("broken name = %q, want the reference fallback", broken.Name)
	}
	if broken.Nodes == nil || broken.Edges == nil {
		t.Fatalf("broken nodes/edges = %v/%v, want empty slices", broken.Nodes, broken.Edges)
	}
	encoded, err := json.Marshal(broken)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"nodes":[]`) || !strings.Contains(string(encoded), `"edges":[]`) {
		t.Fatalf("broken entry does not encode empty arrays: %s", encoded)
	}

	good := catalogEntry(t, catalog, "good")
	if !good.Valid {
		t.Fatalf("good workflow is invalid: %v", good.Problems)
	}
}

func TestReadWorkflowCatalogListsStructurallyInvalidFiles(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "duplicate.yaml"), `
id: duplicate
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent}
  - {id: a, kind: agent}
`)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "cycle.yaml"), `
id: cycle
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent}
  - {id: b, kind: agent}
edges:
  - {from: a, to: b}
  - {from: b, to: a}
`)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "ok.yaml"), `
id: ok
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent}
`)

	catalog, err := ReadWorkflowCatalog(context.Background(), project)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if len(catalog) != 3 {
		t.Fatalf("catalog = %+v, want three entries", catalog)
	}

	duplicate := catalogEntry(t, catalog, "duplicate")
	if duplicate.Valid || !containsProblem(duplicate.Problems, "duplicate node id") {
		t.Fatalf("duplicate entry = valid:%v problems:%v", duplicate.Valid, duplicate.Problems)
	}

	cyclic := catalogEntry(t, catalog, "cycle")
	if cyclic.Valid || !containsProblem(cyclic.Problems, "cycle") {
		t.Fatalf("cycle entry = valid:%v problems:%v", cyclic.Valid, cyclic.Problems)
	}
	if len(cyclic.Nodes) != 2 || len(cyclic.Edges) != 2 {
		t.Fatalf("cycle entry lost its topology: %+v", cyclic)
	}

	ok := catalogEntry(t, catalog, "ok")
	if !ok.Valid {
		t.Fatalf("valid entry was marked invalid: %v", ok.Problems)
	}
}

func TestReadWorkflowCatalogKeepsMissingSkillValid(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "unresolved.yaml"), `
id: unresolved
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent, skill: definitely-not-installed}
`)

	catalog, err := ReadWorkflowCatalog(context.Background(), project)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	entry := catalogEntry(t, catalog, "unresolved")
	if !entry.Valid {
		t.Fatalf("a missing skill invalidated the listing: %v", entry.Problems)
	}
	if len(entry.Problems) != 0 {
		t.Fatalf("a missing skill produced problems: %v", entry.Problems)
	}
}

func TestReadWorkflowCatalogReadsAFolderProject(t *testing.T) {
	directory := t.TempDir()

	catalog, err := ReadWorkflowCatalog(context.Background(), directory)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(catalog) != 0 {
		t.Fatalf("catalog = %+v, want empty", catalog)
	}
}

func TestReadWorkflowCatalogChangesNothing(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "one.yaml"), catalogWorkflow)
	writeTestFile(t, filepath.Join(project, "workflows", "two.yaml"), `
id: two
name: Two
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent}
`)
	writeTestFile(t, filepath.Join(project, ".specflow", "runs", "existing", "run.json"), `{"id":"existing"}`)

	before := snapshotProjectFiles(t, project)

	if _, err := ReadWorkflowCatalog(context.Background(), project); err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	after := snapshotProjectFiles(t, project)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("catalog read changed the project\nbefore: %v\nafter:  %v", before, after)
	}
	if _, err := os.Stat(filepath.Join(project, ".specflow", "worktrees")); !os.IsNotExist(err) {
		t.Fatalf("catalog read created a worktree directory: %v", err)
	}
}

// snapshotProjectFiles captures every file outside .git so a test can prove a
// read created, changed, or removed nothing.
func snapshotProjectFiles(t *testing.T, root string) map[string]string {
	t.Helper()

	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[relative] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot project: %v", err)
	}
	return files
}

func containsProblem(problems []string, fragment string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, fragment) {
			return true
		}
	}
	return false
}
