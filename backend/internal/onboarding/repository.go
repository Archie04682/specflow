package onboarding

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"example.com/specflow/backend/internal/workspace"
)

// ErrAlreadyRepository reports a directory that already belongs to a git
// repository, including one nested inside another.
var ErrAlreadyRepository = errors.New("project is already a git repository")

// InitResult names the branch a repository was initialised on.
type InitResult struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

// InitRepository initialises a repository in the project directory. It creates
// no commit: staging the operator's files and authoring them is their decision,
// not the server's.
func InitRepository(ctx context.Context, projectPath string) (InitResult, error) {
	root, err := projectDir(projectPath)
	if err != nil {
		return InitResult{}, err
	}

	if toplevel, err := runGit(ctx, root, "rev-parse", "--show-toplevel"); err == nil {
		return InitResult{}, fmt.Errorf("%w: %s", ErrAlreadyRepository, toplevel)
	}

	branch, err := initialise(ctx, root)
	if err != nil {
		return InitResult{}, err
	}
	return InitResult{Path: root, Branch: branch}, nil
}

// initialise runs git init, naming the branch main where git supports it.
func initialise(ctx context.Context, root string) (string, error) {
	if _, err := runGit(ctx, root, "init", "-b", "main"); err == nil {
		return "main", nil
	}

	// Older git has no -b; name the branch after the fact instead.
	if _, err := runGit(ctx, root, "init"); err != nil {
		return "", fmt.Errorf("initialise repository in %s: %w", root, err)
	}
	if _, err := runGit(ctx, root, "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		return "", fmt.Errorf("name the initial branch in %s: %w", root, err)
	}
	return "main", nil
}

// runGit runs one git command in a directory and returns its trimmed output.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf(
			"git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

// ReadRepository reports whether a directory is a repository, its branch, and
// whether it has a commit. It is the read the project tree needs: the full
// readiness report also probes tools and harnesses, which a nav row does not.
func ReadRepository(ctx context.Context, projectPath string) (GitStatus, error) {
	root, err := projectDir(projectPath)
	if err != nil {
		return GitStatus{}, err
	}

	project, err := workspace.Open(ctx, root)
	if err != nil {
		return GitStatus{}, err
	}
	// A directory that is not a repository is the answer, not a failure.
	return project.GitStatus(ctx)
}
