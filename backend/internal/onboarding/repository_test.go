package onboarding

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInitRepositoryCreatesTheRepository(t *testing.T) {
	directory := t.TempDir()
	writeFile(t, filepath.Join(directory, "README.md"), "hello\n")

	result, err := InitRepository(context.Background(), directory)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if result.Branch != "main" {
		t.Fatalf("branch = %q", result.Branch)
	}
	if info, err := os.Stat(filepath.Join(directory, ".git")); err != nil || !info.IsDir() {
		t.Fatalf(".git was not created: %v", err)
	}
	// The operator's files are untouched and nothing was committed for them.
	if data, err := os.ReadFile(filepath.Join(directory, "README.md")); err != nil || string(data) != "hello\n" {
		t.Fatalf("README changed: %q %v", data, err)
	}
	if err := exec.Command("git", "-C", directory, "rev-parse", "HEAD").Run(); err == nil {
		t.Fatal("initialising a repository made a commit")
	}
	if branch := runGitOutput(t, directory, "symbolic-ref", "--short", "HEAD"); branch != "main" {
		t.Fatalf("branch = %q", branch)
	}
}

func TestInitRepositoryRefusesAnExistingRepository(t *testing.T) {
	project := newGitProject(t)

	_, err := InitRepository(context.Background(), project)
	if !errors.Is(err, ErrAlreadyRepository) {
		t.Fatalf("init = %v, want ErrAlreadyRepository", err)
	}
}

func TestInitRepositoryRefusesANestedDirectory(t *testing.T) {
	project := newGitProject(t)
	nested := filepath.Join(project, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := InitRepository(context.Background(), nested)
	if !errors.Is(err, ErrAlreadyRepository) {
		t.Fatalf("init = %v, want ErrAlreadyRepository", err)
	}
}

func TestInitRepositoryRefusesAMissingPath(t *testing.T) {
	_, err := InitRepository(context.Background(), filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("a missing path was initialised")
	}
}

// runGitOutput reads one git value, failing the test on error.
func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()

	output, err := runGit(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return output
}

func TestReadRepository(t *testing.T) {
	committed := newGitProject(t)
	status, err := ReadRepository(context.Background(), committed)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !status.Repository || status.Branch != "main" || !status.HasCommit {
		t.Fatalf("committed project = %+v", status)
	}

	plain := t.TempDir()
	status, err = ReadRepository(context.Background(), plain)
	if err != nil {
		t.Fatalf("read plain: %v", err)
	}
	if status.Repository || status.Reason == "" {
		t.Fatalf("plain directory = %+v", status)
	}

	initialised := t.TempDir()
	if _, err := InitRepository(context.Background(), initialised); err != nil {
		t.Fatalf("init: %v", err)
	}
	status, err = ReadRepository(context.Background(), initialised)
	if err != nil {
		t.Fatalf("read initialised: %v", err)
	}
	if !status.Repository || status.HasCommit || status.Branch != "main" {
		t.Fatalf("initialised project = %+v", status)
	}

	if _, err := ReadRepository(context.Background(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing path was reported")
	}
}
