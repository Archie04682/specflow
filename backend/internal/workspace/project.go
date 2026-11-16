// Package workspace manages the git-isolated workspace a run executes in and
// the per-project .specflow metadata directory.
//
// A project is the user's repository. A run never executes in it directly:
// every run gets a worktree on its own branch so the user's checkout is left
// untouched, and the project itself only ever gains the .specflow directory.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	specflowDirName  = ".specflow"
	runsDirName      = "runs"
	worktreesDirName = "worktrees"
	configFileName   = "config.yaml"
	gitignoreName    = ".gitignore"
)

// ErrNotGitRepository reports a project directory that git does not track.
var ErrNotGitRepository = errors.New("project is not a git repository")

// ErrDetachedHEAD reports a project whose HEAD is not on a branch, so there is
// no source branch a run could merge back into.
var ErrDetachedHEAD = errors.New("project has a detached HEAD")

// Project is a user's git repository.
type Project struct {
	Root string
}

// Open resolves a project root and verifies that it is a git repository.
func Open(ctx context.Context, root string) (*Project, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	// A temporary directory is often reached through a symlink; git records
	// the resolved path in its worktree list, so resolve it here too or the
	// paths we compute will not match the ones git reports.
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("project root %s: %w", absolute, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project root %s is not a directory", absolute)
	}

	project := &Project{Root: absolute}
	if _, err := project.Git(ctx, "rev-parse", "--show-toplevel"); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotGitRepository, absolute)
	}

	return project, nil
}

// Git runs a git command in the project root and returns trimmed stdout.
func (p *Project) Git(ctx context.Context, args ...string) (string, error) {
	return runGit(ctx, p.Root, args...)
}

// CurrentBranch returns the checked-out branch name, or ErrDetachedHEAD.
func (p *Project) CurrentBranch(ctx context.Context) (string, error) {
	branch, err := p.Git(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", ErrDetachedHEAD
	}
	return branch, nil
}

// Head returns the commit currently checked out.
func (p *Project) Head(ctx context.Context) (string, error) {
	return p.Git(ctx, "rev-parse", "HEAD")
}

// Uncommitted lists the paths that differ from HEAD, ignoring the .specflow
// directory: run metadata is expected to be uncommitted and must not make the
// next run look dirty.
func (p *Project) Uncommitted(ctx context.Context) ([]string, error) {
	output, err := p.Git(ctx, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	return filterStatus(output), nil
}

// CheckIsolatable refuses a project that a worktree cannot be branched from
// cleanly. The report is a list of offending paths, not a git transcript.
func (p *Project) CheckIsolatable(ctx context.Context) error {
	paths, err := p.Uncommitted(ctx)
	if err != nil {
		return err
	}
	if len(paths) > 0 {
		return fmt.Errorf("project has uncommitted changes: %s", strings.Join(paths, ", "))
	}
	return nil
}

// SpecflowDir is the per-project metadata directory.
func (p *Project) SpecflowDir() string {
	return filepath.Join(p.Root, specflowDirName)
}

// RunDir is where one run's metadata is stored.
func (p *Project) RunDir(runID string) string {
	return filepath.Join(p.SpecflowDir(), runsDirName, runID)
}

// RunFile is the durable record of a run.
func (p *Project) RunFile(runID string) string {
	return filepath.Join(p.RunDir(runID), "run.json")
}

// WorkflowSnapshotFile is the workflow definition a run was compiled from.
func (p *Project) WorkflowSnapshotFile(runID string) string {
	return filepath.Join(p.RunDir(runID), "workflow.yaml")
}

// RunBranch is the branch a run executes on.
func (p *Project) RunBranch(runID string) string {
	return "specflow/run-" + runID
}

// EventsFile is the append-only event log of a run.
func (p *Project) EventsFile(runID string) string {
	return filepath.Join(p.RunDir(runID), "events.jsonl")
}

// WorktreeDir is where one run's checkout lives.
func (p *Project) WorktreeDir(runID string) string {
	return filepath.Join(p.SpecflowDir(), worktreesDirName, runID)
}

// EnsureLayout creates the metadata directory, the ignore rule that keeps
// worktrees out of version control, and a default configuration file.
func (p *Project) EnsureLayout() error {
	if err := os.MkdirAll(filepath.Join(p.SpecflowDir(), runsDirName), 0o755); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(p.SpecflowDir(), worktreesDirName), 0o755); err != nil {
		return fmt.Errorf("create worktree directory: %w", err)
	}

	ignorePath := filepath.Join(p.SpecflowDir(), gitignoreName)
	if _, err := os.Stat(ignorePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(ignorePath, []byte(worktreesDirName+"/\n"), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", ignorePath, err)
		}
	}

	configPath := filepath.Join(p.SpecflowDir(), configFileName)
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(configPath, defaultConfig, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", configPath, err)
		}
	}

	return nil
}

var defaultConfig = []byte(`# SpecFlow project configuration.
#
# skills lists the directories searched for skill names, in order. Relative
# paths resolve against the project root. When the list is empty, the defaults
# below are used.
skills: []
#  - .specflow/skills
#  - .dsh/skills
#  - .pi/skills
#  - ~/.dsh/skills
`)

// Config is the subset of project configuration this change reads.
type Config struct {
	Skills []string `yaml:"skills"`
}

// LoadConfig reads .specflow/config.yaml. A missing file is an empty config.
func (p *Project) LoadConfig() (Config, error) {
	path := filepath.Join(p.SpecflowDir(), configFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return config, nil
}

// defaultSkillRoots is ordered by precedence. The project's shared folder wins
// so one copy of a skill can serve every harness; the harness-specific folders
// stay supported for projects that already keep skills there.
var defaultSkillRoots = []string{".specflow/skills", ".dsh/skills", ".pi/skills", "~/.dsh/skills"}

// SkillSource is one configured root: the label shown to a person and the
// resolved absolute directory.
type SkillSource struct {
	Label string
	Path  string
}

// SkillSources returns the configured skill roots in resolution order.
func (p *Project) SkillSources() ([]SkillSource, error) {
	config, err := p.LoadConfig()
	if err != nil {
		return nil, err
	}

	labels := config.Skills
	if len(labels) == 0 {
		labels = defaultSkillRoots
	}

	sources := make([]SkillSource, 0, len(labels))
	for _, label := range labels {
		sources = append(sources, SkillSource{
			Label: label,
			Path:  expandRoot(p.Root, label),
		})
	}
	return sources, nil
}

// SkillRoots returns the directories a skill name is resolved against.
func (p *Project) SkillRoots() ([]string, error) {
	sources, err := p.SkillSources()
	if err != nil {
		return nil, err
	}

	roots := make([]string, 0, len(sources))
	for _, source := range sources {
		roots = append(roots, source.Path)
	}
	return roots, nil
}

func expandRoot(projectRoot, root string) string {
	if strings.HasPrefix(root, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(root, "~/"))
		}
	}
	if filepath.IsAbs(root) {
		return root
	}
	return filepath.Join(projectRoot, root)
}

// filterStatus turns porcelain output into paths, dropping the metadata
// directory that this package itself maintains.
func filterStatus(output string) []string {
	var paths []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		path := statusPath(line)
		if path == "" {
			continue
		}
		// An untracked directory is reported with a trailing slash.
		path = strings.TrimSuffix(path, "/")
		if path == specflowDirName || strings.HasPrefix(path, specflowDirName+"/") {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func statusPath(line string) string {
	if len(line) < 3 {
		return ""
	}
	path := strings.TrimSpace(line[3:])
	if before, after, found := strings.Cut(path, " -> "); found {
		_, _ = before, after
		path = after
	}
	return strings.Trim(path, `"`)
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir

	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}
	return strings.TrimSpace(stdout.String()), nil
}
