package onboarding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitStagesAndCommitsEverything(t *testing.T) {
	project := newGitProject(t)
	writeFile(t, filepath.Join(project, "notes.md"), "notes\n")

	result, err := Commit(context.Background(), project, "")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if result.Commit == "" || len(result.Paths) != 1 || result.Paths[0] != "notes.md" {
		t.Fatalf("result = %+v", result)
	}
	status, err := os.ReadFile(filepath.Join(project, "notes.md"))
	if err != nil || string(status) != "notes\n" {
		t.Fatalf("committed file changed: %v %q", err, status)
	}

	if _, err := Commit(context.Background(), project, ""); !errors.Is(err, ErrNothingToCommit) {
		t.Fatalf("second commit: %v", err)
	}
}

func TestCommitRefusesSecretLookingPaths(t *testing.T) {
	project := newGitProject(t)
	writeFile(t, filepath.Join(project, ".env"), "TOKEN=secret\n")
	writeFile(t, filepath.Join(project, ".env.example"), "TOKEN=\n")

	var secrets *SecretPathsError
	_, err := Commit(context.Background(), project, "")
	if !errors.As(err, &secrets) {
		t.Fatalf("err = %v, want SecretPathsError", err)
	}
	if len(secrets.Paths) != 1 || secrets.Paths[0] != ".env" {
		t.Fatalf("secrets = %v", secrets.Paths)
	}
}

func TestIgnoreAddsAPathOnce(t *testing.T) {
	project := newGitProject(t)
	writeFile(t, filepath.Join(project, ".gitignore"), "node_modules/\n")

	result, err := Ignore(context.Background(), project, []string{".env", ".env"})
	if err != nil {
		t.Fatalf("ignore: %v", err)
	}
	if len(result.Ignored) != 1 || result.Ignored[0] != ".env" {
		t.Fatalf("ignored = %v", result.Ignored)
	}

	data, err := os.ReadFile(filepath.Join(project, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), ".env") != 1 {
		t.Fatalf(".gitignore = %q", data)
	}

	again, err := Ignore(context.Background(), project, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Ignored) != 0 {
		t.Fatalf("re-ignore = %v", again.Ignored)
	}
}

func TestIgnoreRefusesAPathOutsideTheProject(t *testing.T) {
	project := newGitProject(t)

	for _, path := range []string{"../elsewhere", "/etc/passwd", ".gitignore"} {
		if _, err := Ignore(context.Background(), project, []string{path}); err == nil {
			t.Fatalf("ignore accepted %q", path)
		}
	}
}

func TestCommitFallsBackWhenTheMachineHasNoIdentity(t *testing.T) {
	project := newGitProject(t)
	gitRun(t, project, "config", "--unset", "user.name")
	gitRun(t, project, "config", "--unset", "user.email")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("HOME", t.TempDir())
	// Without a configured identity git may still auto-detect one; this forces
	// the "Author identity unknown" path the fallback exists for.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "user.useConfigOnly")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")

	writeFile(t, filepath.Join(project, "notes.md"), "notes\n")
	result, err := Commit(context.Background(), project, "Initial commit")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !result.AuthorFallback {
		t.Fatalf("result = %+v, want the fallback author", result)
	}
}

func TestSecretPaths(t *testing.T) {
	paths := []string{
		".env",
		".env.local",
		".env.example",
		"config/pem/key.pem",
		"id_rsa",
		"README.md",
		"credentials.json",
	}
	got := SecretPaths(paths)
	want := []string{".env", ".env.local", "config/pem/key.pem", "id_rsa", "credentials.json"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("secrets = %v, want %v", got, want)
	}
}
