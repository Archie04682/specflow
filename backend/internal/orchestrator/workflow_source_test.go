package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// validSourceDraft is a structurally valid workflow that needs no registered
// harness to validate.
func validSourceDraft() WorkflowDraft {
	return WorkflowDraft{
		ID:      "demo",
		Name:    "Demo",
		Version: 1,
		Defaults: WorkflowDefaultsDraft{
			Harness: "fake",
		},
		Nodes: []WorkflowNodeDraft{
			{ID: "explore", Title: "Explore", Kind: NodeKindAgent, Prompt: "look around"},
		},
		Edges: []WorkflowEdgeDraft{},
	}
}

func readWorkflowTestFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func hashWorkflowTestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// changedProjectFiles reports every file that appeared, disappeared, or changed
// content between two project snapshots.
func changedProjectFiles(before, after map[string]string) []string {
	changed := make([]string, 0)
	for path, content := range after {
		if previous, ok := before[path]; !ok || previous != content {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func TestReadWorkflowSourcePrefersTheProjectCopy(t *testing.T) {
	project := newRepoProject(t)
	projectCopy := `
id: project-copy
name: Project copy
defaults: {harness: fake}
nodes:
  - {id: project, kind: agent}
`
	template := `
id: template
name: Template
defaults: {harness: fake}
nodes:
  - {id: template, kind: agent}
`
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "shared.yaml"), projectCopy)
	writeTestFile(t, filepath.Join(project, "workflows", "shared.yaml"), template)

	source, err := ReadWorkflowSource(context.Background(), project, "shared")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	wantPath := filepath.Join(project, ".specflow", "workflows", "shared.yaml")
	if source.Reference != "shared" || source.Path != wantPath {
		t.Fatalf("source = %q at %q, want shared at %q", source.Reference, source.Path, wantPath)
	}
	if source.IsTemplate {
		t.Fatal("the project copy was reported as a template")
	}
	if source.Source != projectCopy {
		t.Fatalf("source bytes = %q, want the project copy", source.Source)
	}
	if want := hashWorkflowTestBytes([]byte(projectCopy)); source.Hash != want {
		t.Fatalf("hash = %q, want %q", source.Hash, want)
	}
	if source.Draft.ID != "project-copy" {
		t.Fatalf("draft id = %q, want the project copy", source.Draft.ID)
	}
	if len(source.Draft.Nodes) != 1 || source.Draft.Nodes[0].ID != "project" {
		t.Fatalf("draft nodes = %+v, want the project copy", source.Draft.Nodes)
	}
	if source.Draft.Edges == nil {
		t.Fatal("draft edges are nil, want an empty array")
	}
}

func TestReadWorkflowSourceFromTemplate(t *testing.T) {
	project := newRepoProject(t)
	template := `
id: template
name: Template
defaults: {harness: fake}
nodes:
  - {id: only, kind: agent}
`
	writeTestFile(t, filepath.Join(project, "workflows", "template.yaml"), template)

	source, err := ReadWorkflowSource(context.Background(), project, "template")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	wantPath := filepath.Join(project, "workflows", "template.yaml")
	if source.Path != wantPath {
		t.Fatalf("path = %q, want %q", source.Path, wantPath)
	}
	if !source.IsTemplate {
		t.Fatal("a workflows/ file was not reported as a template")
	}
	if source.Draft.ID != "template" || len(source.Draft.Nodes) != 1 {
		t.Fatalf("draft = %+v", source.Draft)
	}
}

func TestReadWorkflowSourceKeepsAnInvalidGraphOpenable(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "broken-graph.yaml"), `
id: broken-graph
defaults: {harness: fake}
nodes:
  - {id: a, kind: agent}
  - {id: a, kind: agent}
`)

	// A structural defect is editable, so opening it must not fail.
	source, err := ReadWorkflowSource(context.Background(), project, "broken-graph")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if len(source.Draft.Nodes) != 2 {
		t.Fatalf("draft nodes = %+v, want the authored duplicate", source.Draft.Nodes)
	}
}

func TestReadWorkflowSourceUnknownReference(t *testing.T) {
	project := newRepoProject(t)

	_, err := ReadWorkflowSource(context.Background(), project, "missing")
	if !errors.Is(err, ErrWorkflowNotFound) {
		t.Fatalf("error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestReadWorkflowSourceRejectsInvalidReferences(t *testing.T) {
	// A non-git directory proves the reference is checked before the project is
	// touched: an invalid reference never reaches a path.
	project := t.TempDir()

	references := []string{
		"",
		"../escape",
		"a/b",
		`a\b`,
		".hidden",
		"..",
		".",
		"a b",
		" work",
		strings.Repeat("a", 129),
	}
	for _, reference := range references {
		t.Run(reference, func(t *testing.T) {
			_, err := ReadWorkflowSource(context.Background(), project, reference)
			if !errors.Is(err, ErrInvalidWorkflowReference) {
				t.Fatalf("error = %v, want ErrInvalidWorkflowReference", err)
			}
		})
	}
}

func TestWorkflowReferenceAcceptsBareFilenames(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "a.b_c-1.yaml"), `
id: a.b_c-1
defaults: {harness: fake}
nodes:
  - {id: only, kind: agent}
`)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", strings.Repeat("a", 128)+".yaml"), `
id: long
defaults: {harness: fake}
nodes:
  - {id: only, kind: agent}
`)

	for _, reference := range []string{"a.b_c-1", strings.Repeat("a", 128)} {
		if _, err := ReadWorkflowSource(context.Background(), project, reference); err != nil {
			t.Fatalf("reference %q was rejected: %v", reference, err)
		}
	}
}

func TestReadWorkflowSourceUnparseableFile(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, ".specflow", "workflows", "broken.yaml"), "nodes: [\n  - {\n")

	_, err := ReadWorkflowSource(context.Background(), project, "broken")
	if err == nil {
		t.Fatal("an unparseable workflow was accepted")
	}
	if errors.Is(err, ErrWorkflowNotFound) || errors.Is(err, ErrInvalidWorkflowReference) {
		t.Fatalf("parse error was misclassified: %v", err)
	}
	if !strings.Contains(err.Error(), "parse workflow") {
		t.Fatalf("error = %v, want a parse error", err)
	}
}

func TestReadWorkflowSourceInvalidProject(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := ReadWorkflowSource(context.Background(), missing, "anything")
	if err == nil {
		t.Fatal("a missing project was accepted")
	}
	if !errors.Is(err, ErrWorkflowProject) {
		t.Fatalf("error = %v, want ErrWorkflowProject", err)
	}
}

func TestReadWorkflowSourceAcceptsAFolderProject(t *testing.T) {
	_, err := ReadWorkflowSource(context.Background(), t.TempDir(), "missing-flow")
	if !errors.Is(err, ErrWorkflowNotFound) {
		t.Fatalf("error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestSaveWorkflowSourceCreatesTheProjectCopy(t *testing.T) {
	project := newRepoProject(t)
	draft := validSourceDraft()

	path, hash, err := SaveWorkflowSource(context.Background(), project, "created", draft, "")
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	wantPath := filepath.Join(project, ".specflow", "workflows", "created.yaml")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	data := readWorkflowTestFile(t, path)
	if want := hashWorkflowTestBytes(data); hash != want {
		t.Fatalf("returned hash = %q, want %q", hash, want)
	}
	if !strings.HasPrefix(string(data), "# Managed by the SpecFlow workflow editor.") {
		t.Fatalf("saved file has no managed header:\n%s", data)
	}

	source, err := ReadWorkflowSource(context.Background(), project, "created")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if source.Hash != hash || source.Source != string(data) {
		t.Fatalf("read back source/hash = %q/%q", source.Source, source.Hash)
	}
	if !reflect.DeepEqual(source.Draft, draft) {
		t.Fatalf("draft did not round-trip:\n got: %+v\nwant: %+v", source.Draft, draft)
	}
}

func TestSaveWorkflowSourceUpdatesAnExistingFile(t *testing.T) {
	project := newRepoProject(t)
	draft := validSourceDraft()

	path, createdHash, err := SaveWorkflowSource(context.Background(), project, "flow", draft, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	before := readWorkflowTestFile(t, path)

	updated := draft
	updated.Name = "Renamed"
	updated.Nodes = append(updated.Nodes, WorkflowNodeDraft{
		ID: "apply", Title: "Apply", Kind: NodeKindAgent, Prompt: "implement",
	})
	updated.Edges = append(updated.Edges, WorkflowEdgeDraft{From: "explore", To: "apply"})

	_, updatedHash, err := SaveWorkflowSource(context.Background(), project, "flow", updated, createdHash)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updatedHash == createdHash {
		t.Fatal("update returned the previous hash")
	}
	after := readWorkflowTestFile(t, path)
	if string(before) == string(after) {
		t.Fatal("update did not change the file")
	}

	source, err := ReadWorkflowSource(context.Background(), project, "flow")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if source.Hash != updatedHash {
		t.Fatalf("read hash = %q, want %q", source.Hash, updatedHash)
	}
	if source.Draft.Name != "Renamed" || len(source.Draft.Edges) != 1 {
		t.Fatalf("draft = %+v", source.Draft)
	}
}

func TestSaveWorkflowSourceCopiesATemplateWithoutTouchingIt(t *testing.T) {
	project := newRepoProject(t)
	templatePath := filepath.Join(project, "workflows", "template.yaml")
	template := `
id: template
name: Template
defaults: {harness: fake}
nodes:
  - {id: only, kind: agent}
`
	writeTestFile(t, templatePath, template)

	source, err := ReadWorkflowSource(context.Background(), project, "template")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	if !source.IsTemplate {
		t.Fatal("template was not flagged")
	}

	path, hash, err := SaveWorkflowSource(context.Background(), project, "template", source.Draft, source.Hash)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	wantPath := filepath.Join(project, ".specflow", "workflows", "template.yaml")
	if path != wantPath {
		t.Fatalf("path = %q, want the project copy %q", path, wantPath)
	}
	if data := readWorkflowTestFile(t, templatePath); string(data) != template {
		t.Fatalf("template was modified:\n%s", data)
	}
	if data := readWorkflowTestFile(t, path); hashWorkflowTestBytes(data) != hash {
		t.Fatalf("returned hash does not match the written bytes")
	}
}

func TestSaveWorkflowSourceRefusesAStaleBaseHash(t *testing.T) {
	project := newRepoProject(t)
	draft := validSourceDraft()

	path, hash, err := SaveWorkflowSource(context.Background(), project, "stale", draft, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Someone else edits the file after the editor opened it.
	concurrent := validSourceDraft()
	concurrent.Name = "Edited elsewhere"
	writeTestFile(t, path, "id: concurrent\n")
	changed := readWorkflowTestFile(t, path)

	_, _, err = SaveWorkflowSource(context.Background(), project, "stale", draft, hash)
	var conflict *WorkflowConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want *WorkflowConflictError", err)
	}
	if want := hashWorkflowTestBytes(changed); conflict.Hash != want {
		t.Fatalf("conflict hash = %q, want %q", conflict.Hash, want)
	}
	if conflict.Error() != "workflow changed on disk" {
		t.Fatalf("conflict message = %q", conflict.Error())
	}
	if data := readWorkflowTestFile(t, path); string(data) != string(changed) {
		t.Fatalf("a refused save changed the file:\n%s", data)
	}
}

func TestSaveWorkflowSourceRefusesAnEmptyBaseHashOverAFile(t *testing.T) {
	project := newRepoProject(t)
	draft := validSourceDraft()

	_, hash, err := SaveWorkflowSource(context.Background(), project, "existing", draft, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, _, err = SaveWorkflowSource(context.Background(), project, "existing", draft, "")
	var conflict *WorkflowConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want *WorkflowConflictError", err)
	}
	if conflict.Hash != hash {
		t.Fatalf("conflict hash = %q, want %q", conflict.Hash, hash)
	}
}

func TestSaveWorkflowSourceConflictWhenTheOpenedFileIsGone(t *testing.T) {
	project := newRepoProject(t)

	// The client opened a template, but that template no longer exists.
	_, _, err := SaveWorkflowSource(context.Background(), project, "vanished", validSourceDraft(), hashWorkflowTestBytes([]byte("gone")))
	var conflict *WorkflowConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want *WorkflowConflictError", err)
	}
	if conflict.Hash != "" {
		t.Fatalf("conflict hash = %q, want empty", conflict.Hash)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".specflow", "workflows", "vanished.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("a conflicted save wrote a file: %v", statErr)
	}
}

func TestSaveWorkflowSourceRejectsStructuralProblems(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*WorkflowDraft)
		contains string
	}{
		{
			name:     "no nodes",
			mutate:   func(draft *WorkflowDraft) { draft.Nodes = nil },
			contains: "no nodes",
		},
		{
			name: "cycle",
			mutate: func(draft *WorkflowDraft) {
				draft.Nodes = []WorkflowNodeDraft{
					{ID: "a", Kind: NodeKindAgent},
					{ID: "b", Kind: NodeKindAgent},
				}
				draft.Edges = []WorkflowEdgeDraft{{From: "a", To: "b"}, {From: "b", To: "a"}}
			},
			contains: "cycle",
		},
		{
			name: "duplicate node id",
			mutate: func(draft *WorkflowDraft) {
				draft.Nodes = []WorkflowNodeDraft{
					{ID: "same", Kind: NodeKindAgent},
					{ID: "same", Kind: NodeKindAgent},
				}
			},
			contains: "duplicate node id",
		},
		{
			name: "agent without a harness",
			mutate: func(draft *WorkflowDraft) {
				draft.Defaults.Harness = ""
				draft.Nodes = []WorkflowNodeDraft{{ID: "a", Kind: NodeKindAgent}}
			},
			contains: "names no harness",
		},
		{
			name: "dangling edge",
			mutate: func(draft *WorkflowDraft) {
				draft.Edges = []WorkflowEdgeDraft{{From: "explore", To: "ghost"}}
			},
			contains: `unknown node "ghost"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := newRepoProject(t)
			draft := validSourceDraft()
			test.mutate(&draft)

			_, _, err := SaveWorkflowSource(context.Background(), project, "invalid", draft, "")
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v (%T), want *ValidationError", err, err)
			}
			if !containsProblem(validation.Problems, test.contains) {
				t.Fatalf("problems = %v, want one containing %q", validation.Problems, test.contains)
			}
			if _, statErr := os.Stat(filepath.Join(project, ".specflow", "workflows", "invalid.yaml")); !os.IsNotExist(statErr) {
				t.Fatalf("an invalid draft was written: %v", statErr)
			}
		})
	}
}

func TestSaveWorkflowSourceReturnsTheValidationErrorUnchanged(t *testing.T) {
	project := newRepoProject(t)
	draft := validSourceDraft()
	draft.Nodes = nil

	_, _, err := SaveWorkflowSource(context.Background(), project, "invalid", draft, "")
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("error = %T, want the *ValidationError unchanged", err)
	}
}

func TestSaveWorkflowSourceRejectsAnInvalidReference(t *testing.T) {
	// The project is deliberately not a git repository: the reference must be
	// rejected before the project is opened or a path is touched.
	project := t.TempDir()
	before := snapshotProjectFiles(t, project)

	_, _, err := SaveWorkflowSource(context.Background(), project, "../escape", validSourceDraft(), "")
	if !errors.Is(err, ErrInvalidWorkflowReference) {
		t.Fatalf("error = %v, want ErrInvalidWorkflowReference", err)
	}
	if changed := changedProjectFiles(before, snapshotProjectFiles(t, project)); len(changed) != 0 {
		t.Fatalf("an invalid reference changed the project: %v", changed)
	}
}

func TestSaveWorkflowSourceLeavesAStaleFileByteIdenticalOnFailure(t *testing.T) {
	project := newRepoProject(t)
	draft := validSourceDraft()

	path, hash, err := SaveWorkflowSource(context.Background(), project, "readonly", draft, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	before := readWorkflowTestFile(t, path)

	directory := filepath.Dir(path)
	if err := os.Chmod(directory, 0o555); err != nil {
		t.Fatalf("make directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o755) })

	updated := draft
	updated.Name = "Will not be written"
	_, _, err = SaveWorkflowSource(context.Background(), project, "readonly", updated, hash)
	if err == nil {
		t.Fatal("a save into a read-only directory succeeded")
	}

	after := readWorkflowTestFile(t, path)
	if string(before) != string(after) {
		t.Fatalf("a failed save changed the file:\n%s", after)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("a failed save left a temporary file: %s", entry.Name())
		}
	}
}

func TestSaveWorkflowSourceChangesOnlyTheTargetFile(t *testing.T) {
	project := newRepoProject(t)
	writeTestFile(t, filepath.Join(project, "workflows", "template.yaml"), `
id: template
defaults: {harness: fake}
nodes:
  - {id: only, kind: agent}
`)
	runDir := filepath.Join(project, ".specflow", "runs", "run-1")
	writeTestFile(t, filepath.Join(runDir, "run.json"), `{"id":"run-1","status":"completed"}`)
	writeTestFile(t, filepath.Join(runDir, "workflow.yaml"), "id: pinned\n")
	writeTestFile(t, filepath.Join(project, ".specflow", "config.yaml"), "skills: []\n")

	before := snapshotProjectFiles(t, project)
	draft := validSourceDraft()
	if _, _, err := SaveWorkflowSource(context.Background(), project, "isolated", draft, ""); err != nil {
		t.Fatalf("save: %v", err)
	}
	after := snapshotProjectFiles(t, project)

	want := []string{filepath.Join(".specflow", "workflows", "isolated.yaml")}
	if changed := changedProjectFiles(before, after); !reflect.DeepEqual(changed, want) {
		t.Fatalf("save changed %v, want only %v", changed, want)
	}
}
