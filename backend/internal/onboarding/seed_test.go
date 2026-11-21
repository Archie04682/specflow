package onboarding

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	specflow "example.com/specflow"
	"example.com/specflow/backend/internal/orchestrator"
)

func TestSeedFlowWritesTheEmbeddedTemplate(t *testing.T) {
	project := newGitProject(t)

	result, err := SeedFlow(project, "")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if result.Reference != DefaultFlowReference {
		t.Fatalf("reference = %q", result.Reference)
	}

	want, err := fs.ReadFile(specflow.WorkflowTemplates(), "workflows/"+DefaultFlowReference+".yaml")
	if err != nil {
		t.Fatalf("read embedded template: %v", err)
	}
	written, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read seeded flow: %v", err)
	}
	if string(written) != string(want) {
		t.Fatal("the seeded flow is not the embedded template")
	}

	catalog, err := orchestrator.ReadWorkflowCatalog(t.Context(), project)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(catalog) != 1 || !catalog[0].Valid || catalog[0].Reference != DefaultFlowReference {
		t.Fatalf("catalog = %+v", catalog)
	}
}

func TestSeedFlowRefusesToOverwrite(t *testing.T) {
	project := newGitProject(t)
	first, err := SeedFlow(project, "")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = SeedFlow(project, "")
	if !errors.Is(err, ErrFlowExists) {
		t.Fatalf("second seed = %v, want ErrFlowExists", err)
	}
	after, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a refused seed changed the file")
	}
}

func TestSeedFlowKeepsAProjectCopy(t *testing.T) {
	project := newGitProject(t)
	target := filepath.Join(project, ".specflow", "workflows", DefaultFlowReference+".yaml")
	writeFile(t, target, "id: edited\n")

	_, err := SeedFlow(project, "")
	if !errors.Is(err, ErrFlowExists) {
		t.Fatalf("seed = %v, want ErrFlowExists", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "id: edited\n" {
		t.Fatalf("project copy = %q", got)
	}
}

func TestSeedFlowRejectsAnUnknownTemplate(t *testing.T) {
	project := newGitProject(t)

	_, err := SeedFlow(project, "no-such-flow")
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("seed = %v, want ErrTemplateNotFound", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".specflow", "workflows", "no-such-flow.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("an unknown template wrote a file")
	}
}

func TestSeedFlowWritesTheFolderTemplateWhenTheModeIsFolder(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".specflow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".specflow", "config.yaml"), []byte("vcs: folder\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := SeedFlow(project, "")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	want, err := fs.ReadFile(specflow.WorkflowTemplates(), "workflows/"+DefaultFlowReference+"-folder.yaml")
	if err != nil {
		t.Fatalf("read embedded template: %v", err)
	}
	written, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != string(want) {
		t.Fatal("the seeded flow is not the folder template")
	}
	if strings.Contains(string(written), "workdir: project") {
		t.Fatal("the folder template has a project-directory node")
	}
}
