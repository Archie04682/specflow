package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func newProject(t *testing.T) *Project {
	t.Helper()

	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "initial")

	project, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	return project
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureLayoutCreatesMetadataAndIgnoresWorktrees(t *testing.T) {
	ctx := context.Background()
	project := newProject(t)

	if err := project.EnsureLayout(); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	for _, path := range []string{
		project.SpecflowDir(),
		filepath.Join(project.SpecflowDir(), "runs"),
		filepath.Join(project.SpecflowDir(), "worktrees"),
		filepath.Join(project.SpecflowDir(), "config.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	if _, err := project.CreateWorktree(ctx, "run-1"); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	status, err := project.Git(ctx, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(status, "worktrees") {
		t.Fatalf("worktree contents leaked into git status:\n%s", status)
	}
}

func TestWorkflowSnapshotFileLivesUnderTheRunDirectory(t *testing.T) {
	project := newProject(t)

	want := filepath.Join(project.RunDir("run-1"), "workflow.yaml")
	if got := project.WorkflowSnapshotFile("run-1"); got != want {
		t.Fatalf("snapshot path = %q, want %q", got, want)
	}
}

func TestUncommittedIgnoresMetadataDirectory(t *testing.T) {
	ctx := context.Background()
	project := newProject(t)

	if err := project.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	paths, err := project.Uncommitted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("metadata directory made the project dirty: %v", paths)
	}
}

func TestCreateWorktreeLeavesProjectOnItsBranch(t *testing.T) {
	ctx := context.Background()
	project := newProject(t)
	if err := project.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	before, err := project.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}

	worktree, err := project.CreateWorktree(ctx, "run-2")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	after, err := project.CurrentBranch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("project branch changed: %q -> %q", before, after)
	}

	if worktree.Branch != "specflow/run-run-2" {
		t.Fatalf("branch = %q", worktree.Branch)
	}
	if worktree.Source != "main" {
		t.Fatalf("source = %q", worktree.Source)
	}
	if _, err := os.Stat(worktree.Path); err != nil {
		t.Fatalf("worktree path missing: %v", err)
	}

	head, err := worktree.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if head != worktree.Base {
		t.Fatalf("worktree head %q, base %q", head, worktree.Base)
	}
}

func TestOpenRefusesNonGitDirectory(t *testing.T) {
	_, err := Open(context.Background(), t.TempDir())
	if !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("err = %v, want ErrNotGitRepository", err)
	}
}

func TestCreateWorktreeRefusesDirtyProject(t *testing.T) {
	ctx := context.Background()
	project := newProject(t)
	if err := project.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project.Root, "dirty.txt"), "wip\n")

	_, err := project.CreateWorktree(ctx, "run-3")
	if err == nil {
		t.Fatal("expected a dirty project to be refused")
	}
	if !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("error does not name the condition: %v", err)
	}

	if _, statErr := os.Stat(project.WorktreeDir("run-3")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a worktree was created despite the refusal")
	}
	if stash, _ := project.Git(ctx, "stash", "list"); strings.TrimSpace(stash) != "" {
		t.Fatalf("the project working tree was stashed")
	}
}

func TestCheckpointAndRestore(t *testing.T) {
	ctx := context.Background()
	project := newProject(t)
	if err := project.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	worktree, err := project.CreateWorktree(ctx, "run-4")
	if err != nil {
		t.Fatal(err)
	}

	unchanged, err := worktree.Checkpoint(ctx, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != worktree.Base {
		t.Fatalf("an unchanged node created a commit: %q != %q", unchanged, worktree.Base)
	}

	writeFile(t, filepath.Join(worktree.Path, "added.txt"), "new\n")
	changed, err := worktree.Checkpoint(ctx, "node-2")
	if err != nil {
		t.Fatal(err)
	}
	if changed == worktree.Base {
		t.Fatal("a changed node did not create a commit")
	}

	// An ignored file must survive a restore: clean is given -d, never -x.
	excludePath := filepath.Join(project.Root, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludePath, []byte("secret.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(worktree.Path, "secret.txt"), "ignore me\n")

	if err := worktree.Restore(ctx, unchanged); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if _, err := os.Stat(filepath.Join(worktree.Path, "added.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restore kept a file created after the checkpoint")
	}
	if _, err := os.Stat(filepath.Join(worktree.Path, "secret.txt")); err != nil {
		t.Fatalf("restore removed an ignored file: %v", err)
	}
}

func TestCheckMergeable(t *testing.T) {
	ctx := context.Background()

	t.Run("clean project on the source branch", func(t *testing.T) {
		project := newProject(t)
		if err := project.CheckMergeable(ctx, "main"); err != nil {
			t.Fatalf("expected mergeable: %v", err)
		}
	})

	t.Run("wrong branch is refused", func(t *testing.T) {
		project := newProject(t)
		gitRun(t, project.Root, "checkout", "-q", "-b", "other")

		err := project.CheckMergeable(ctx, "main")
		if err == nil || !strings.Contains(err.Error(), "expected") {
			t.Fatalf("err = %v, want a branch mismatch", err)
		}
	})

	t.Run("dirty project is refused", func(t *testing.T) {
		project := newProject(t)
		writeFile(t, filepath.Join(project.Root, "dirty.txt"), "wip\n")

		err := project.CheckMergeable(ctx, "main")
		if err == nil || !strings.Contains(err.Error(), "uncommitted") {
			t.Fatalf("err = %v, want uncommitted changes", err)
		}
	})
}

func TestRemoveWorktreeKeepsBranch(t *testing.T) {
	ctx := context.Background()
	project := newProject(t)
	if err := project.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	worktree, err := project.CreateWorktree(ctx, "run-5")
	if err != nil {
		t.Fatal(err)
	}

	if err := project.RemoveWorktree(ctx, worktree.Path); err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	if _, err := os.Stat(worktree.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worktree still exists: %v", err)
	}

	branches, err := project.Git(ctx, "branch", "--list", worktree.Branch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(branches) == "" {
		t.Fatal("removing the worktree deleted the run branch")
	}
}

func TestSkillSourcesDefaultToTheSharedProjectRootFirst(t *testing.T) {
	project := newProject(t)

	sources, err := project.SkillSources()
	if err != nil {
		t.Fatalf("skill sources: %v", err)
	}
	if len(sources) != 4 {
		t.Fatalf("sources = %+v, want four", sources)
	}
	if sources[0] != (SkillSource{Label: ".specflow/skills", Path: filepath.Join(project.Root, ".specflow", "skills")}) {
		t.Fatalf("first source = %+v", sources[0])
	}
	if sources[1].Label != ".dsh/skills" || sources[2].Label != ".pi/skills" {
		t.Fatalf("sources = %+v", sources)
	}
}

func TestSkillSourcesConfigOverridesDefaults(t *testing.T) {
	project := newProject(t)
	if err := project.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project.SpecflowDir(), "config.yaml"), "skills:\n  - custom/skills\n  - ~/shared-skills\n")

	sources, err := project.SkillSources()
	if err != nil {
		t.Fatalf("skill sources: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %+v, want two", sources)
	}
	if sources[0].Label != "custom/skills" || sources[0].Path != filepath.Join(project.Root, "custom", "skills") {
		t.Fatalf("sources[0] = %+v", sources[0])
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if sources[1].Path != filepath.Join(home, "shared-skills") {
		t.Fatalf("sources[1] = %+v", sources[1])
	}
}
