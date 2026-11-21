package onboarding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/specflow/backend/internal/workspace"
)

// ErrNothingToCommit reports a project whose tree is already clean.
var ErrNothingToCommit = errors.New("there is nothing to commit")

// SecretPathsError reports paths that look like secrets and must not be
// committed. Ignoring them is the operator's call, so it is refused rather than
// silently skipped.
type SecretPathsError struct {
	Paths []string
}

func (e *SecretPathsError) Error() string {
	return "refusing to commit files that look like secrets: " + strings.Join(e.Paths, ", ")
}

// CommitResult names the commit and what it contains.
type CommitResult struct {
	Commit string   `json:"commit"`
	Paths  []string `json:"paths"`
	// AuthorFallback reports a commit made as SpecFlow because the machine has
	// no git identity configured.
	AuthorFallback bool `json:"authorFallback,omitempty"`
}

// IgnoreResult names the paths that were added to .gitignore.
type IgnoreResult struct {
	Ignored []string `json:"ignored"`
}

// Commit stages and commits the project's changes on request. It never runs on
// its own: the first commit is the operator's decision, and this is the button
// that makes it. Paths that look like secrets are refused so a first commit
// cannot publish an environment file by accident.
func Commit(ctx context.Context, projectPath, message string) (CommitResult, error) {
	root, err := projectDir(projectPath)
	if err != nil {
		return CommitResult{}, err
	}
	project, err := workspace.Open(ctx, root)
	if err != nil {
		return CommitResult{}, err
	}
	if mode, err := project.Mode(); err == nil && mode == workspace.ModeFolder {
		return CommitResult{}, errors.New("a folder project has no repository to commit to")
	}
	if err := project.RequireGit(ctx); err != nil {
		return CommitResult{}, err
	}

	paths, err := project.Uncommitted(ctx)
	if err != nil {
		return CommitResult{}, fmt.Errorf("read status: %w", err)
	}
	if len(paths) == 0 {
		return CommitResult{}, ErrNothingToCommit
	}
	if secrets := SecretPaths(paths); len(secrets) > 0 {
		return CommitResult{}, &SecretPathsError{Paths: secrets}
	}

	if strings.TrimSpace(message) == "" {
		message = "Commit changes"
		if _, err := project.Head(ctx); err != nil {
			message = "Initial commit"
		}
	}

	if _, err := project.Git(ctx, "add", "-A"); err != nil {
		return CommitResult{}, fmt.Errorf("stage changes: %w", err)
	}

	authorFallback := false
	if _, err := project.Git(ctx, "commit", "-m", message); err != nil {
		if !identityFailure(err) {
			return CommitResult{}, fmt.Errorf("commit: %w", err)
		}
		// A machine without user.name configured must not block the first
		// commit; retry as SpecFlow and report it.
		if _, retryErr := project.Git(ctx,
			"-c", "user.name=SpecFlow",
			"-c", "user.email=specflow@localhost",
			"commit", "-m", message,
		); retryErr != nil {
			return CommitResult{}, fmt.Errorf("commit: %w", retryErr)
		}
		authorFallback = true
	}

	commit, err := project.Head(ctx)
	if err != nil {
		return CommitResult{}, fmt.Errorf("read commit: %w", err)
	}
	return CommitResult{Commit: commit, Paths: paths, AuthorFallback: authorFallback}, nil
}

// Ignore appends paths to the project's .gitignore. It exists so a refused
// secret has an obvious next step; it runs only when the operator asks.
func Ignore(ctx context.Context, projectPath string, paths []string) (IgnoreResult, error) {
	root, err := projectDir(projectPath)
	if err != nil {
		return IgnoreResult{}, err
	}
	project, err := workspace.Open(ctx, root)
	if err != nil {
		return IgnoreResult{}, err
	}
	if err := project.RequireGit(ctx); err != nil {
		return IgnoreResult{}, err
	}

	clean := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := validateIgnorePath(path); err != nil {
			return IgnoreResult{}, err
		}
		clean = append(clean, filepath.ToSlash(filepath.Clean(path)))
	}
	if len(clean) == 0 {
		return IgnoreResult{}, errors.New("no paths to ignore")
	}

	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return IgnoreResult{}, fmt.Errorf("read .gitignore: %w", err)
	}

	text := string(data)
	present := make(map[string]bool)
	for _, line := range strings.Split(text, "\n") {
		present[strings.TrimSpace(line)] = true
	}

	added := make([]string, 0, len(clean))
	for _, entry := range clean {
		if present[entry] {
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += entry + "\n"
		present[entry] = true
		added = append(added, entry)
	}
	if len(added) > 0 {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return IgnoreResult{}, fmt.Errorf("write .gitignore: %w", err)
		}
	}
	return IgnoreResult{Ignored: added}, nil
}

// SecretPaths returns the paths that must not be committed blindly.
func SecretPaths(paths []string) []string {
	secrets := make([]string, 0)
	for _, path := range paths {
		if looksLikeSecret(filepath.Base(path)) {
			secrets = append(secrets, path)
		}
	}
	return secrets
}

func looksLikeSecret(name string) bool {
	lower := strings.ToLower(name)
	// Example files are meant to be committed.
	for _, suffix := range []string{".example", ".sample", ".template", ".dist"} {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	switch {
	case lower == ".env", strings.HasPrefix(lower, ".env."):
		return true
	case lower == ".npmrc", lower == "credentials.json", lower == "credentials", lower == "secrets.json":
		return true
	case strings.HasPrefix(lower, "id_rsa"), strings.HasPrefix(lower, "id_ed25519"):
		return true
	case strings.HasSuffix(lower, ".pem"), strings.HasSuffix(lower, ".key"),
		strings.HasSuffix(lower, ".p12"), strings.HasSuffix(lower, ".pfx"):
		return true
	}
	return false
}

// validateIgnorePath keeps an ignore request inside the project.
func validateIgnorePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("empty ignore path")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("ignore path must be relative: %s", path)
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("ignore path escapes the project: %s", path)
	}
	if cleaned == ".gitignore" {
		return errors.New("cannot ignore .gitignore")
	}
	return nil
}

// identityFailure reports git's missing-author error.
func identityFailure(err error) bool {
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"author identity unknown",
		"please tell me who you are",
		"unable to auto-detect email address",
		"empty ident name",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
