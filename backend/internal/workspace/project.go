// Package workspace manages the isolated workspace a run executes in and the
// per-project .specflow metadata directory.
//
// A project is the user's directory. A run never executes in it directly: a git
// project gets a worktree on its own branch, and a folder project gets a copy,
// so the user's files are left untouched until the run is applied. The project
// itself only ever gains the .specflow directory.
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

// Workspace modes. A project chooses one; an unset mode is git.
const (
	ModeGit    = "git"
	ModeFolder = "folder"
)

// ErrNotGitRepository reports a project directory that git does not track.
var ErrNotGitRepository = errors.New("project is not a git repository")

// ErrDetachedHEAD reports a project whose HEAD is not on a branch, so there is
// no source branch a run could merge back into.
var ErrDetachedHEAD = errors.New("project has a detached HEAD")

// Project is a user's directory that runs execute from.
type Project struct {
	Root string
}

// Open resolves a project root. The directory need not be a git repository:
// the workspace mode decides how runs are isolated, and the file reads that
// flows, skills, and history need work without one.
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

	return &Project{Root: absolute}, nil
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
#  - .agents/skills
#  - .dsh/skills
#  - .pi/skills
#  - ~/.dsh/skills
#  - ~/.agents/skills
`)

// Config is the subset of project configuration this change reads.
type Config struct {
	Skills []string `yaml:"skills"`
	// VCS is the workspace mode: "git" (the default when unset) or "folder".
	VCS string `yaml:"vcs,omitempty"`
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

// SetMode records the workspace mode in .specflow/config.yaml, editing the
// document rather than re-encoding the struct so comments and unknown keys
// survive. A project may change its mode only before its first run: an existing
// run's copy, snapshots, and apply outcome assume the mode it was created with.
// Re-recording the mode it already has is not a change, so it also works after a
// run: it only makes the choice explicit.
func (p *Project) SetMode(mode string) error {
	switch mode {
	case ModeGit, ModeFolder:
	default:
		return fmt.Errorf("unknown workspace mode %q: use %q or %q", mode, ModeGit, ModeFolder)
	}

	current, err := p.Mode()
	if err != nil {
		return err
	}
	if current != mode {
		runs, err := p.HasRuns()
		if err != nil {
			return err
		}
		if runs {
			return errors.New("the workspace mode cannot change after a run exists")
		}
	}

	path := filepath.Join(p.SpecflowDir(), configFileName)
	var document yaml.Node
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("read %s: %w", path, err)
	default:
		if err := yaml.Unmarshal(data, &document); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}

	if err := setConfigValue(&document, "vcs", mode); err != nil {
		return fmt.Errorf("edit %s: %w", path, err)
	}
	encoded, err := yaml.Marshal(&document)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := writeFileAtomic(path, encoded); err != nil {
		return err
	}
	return nil
}

// setConfigValue sets one key in a YAML document, creating the mapping when the
// document is empty.
func setConfigValue(document *yaml.Node, key, value string) error {
	if document.Kind == 0 {
		document.Kind = yaml.DocumentNode
	}
	if len(document.Content) == 0 {
		document.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return errors.New("config file is not a mapping")
	}
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value == key {
			root.Content[index+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return nil
		}
	}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	return nil
}

// writeFileAtomic replaces a file with new content through a temporary file in
// the same directory, so a crash cannot leave a partial write.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	name := temporary.Name()
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// Mode reports the project's workspace mode. An unset mode is git, so an
// existing project keeps its worktree without a migration.
func (p *Project) Mode() (string, error) {
	config, err := p.LoadConfig()
	if err != nil {
		return "", err
	}
	if config.VCS == "" {
		return ModeGit, nil
	}
	return config.VCS, nil
}

// ModeIsExplicit reports whether the project recorded a mode, rather than
// running on the git default.
func (p *Project) ModeIsExplicit() (bool, error) {
	config, err := p.LoadConfig()
	if err != nil {
		return false, err
	}
	return config.VCS != "", nil
}

// HasRuns reports whether the project has any run metadata.
func (p *Project) HasRuns() (bool, error) {
	entries, err := os.ReadDir(filepath.Join(p.SpecflowDir(), runsDirName))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runs: %w", err)
	}
	return len(entries) > 0, nil
}

// GitStatus is a project's git state. It is reported, not required: a folder
// project is not a repository and is still a valid project.
type GitStatus struct {
	Repository bool   `json:"repository"`
	Branch     string `json:"branch,omitempty"`
	HasCommit  bool   `json:"hasCommit"`
	Reason     string `json:"reason,omitempty"`
	// VCS is the project's workspace mode, so a reader knows whether a missing
	// repository is a gap.
	VCS string `json:"vcs,omitempty"`
}

// GitStatus reports the project's git state without failing for a directory
// that is not a repository.
func (p *Project) GitStatus(ctx context.Context) (GitStatus, error) {
	mode, err := p.Mode()
	if err != nil {
		return GitStatus{}, err
	}

	status := GitStatus{VCS: mode}
	if _, err := p.Git(ctx, "rev-parse", "--is-inside-work-tree"); err != nil {
		status.Reason = "not a git repository"
		return status, nil
	}

	status.Repository = true
	if branch, err := p.CurrentBranch(ctx); err == nil {
		status.Branch = branch
	} else {
		status.Reason = err.Error()
	}
	// An initialised repository with no commit cannot back a worktree, so this
	// is its own fact rather than a branch failure.
	if _, err := p.Head(ctx); err == nil {
		status.HasCommit = true
	}
	return status, nil
}

// RequireGit reports a directory that is not a repository, naming the project.
func (p *Project) RequireGit(ctx context.Context) error {
	if _, err := p.Git(ctx, "rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("%w: %s", ErrNotGitRepository, p.Root)
	}
	return nil
}

// defaultSkillRoots is ordered by precedence: shared project, then
// harness-specific project, then global. `.agents/skills` is where the skills
// CLI installs for most harnesses, so it sits with the project's own shared
// folder rather than with a harness-specific one.
var defaultSkillRoots = []string{
	".specflow/skills",
	".agents/skills",
	".dsh/skills",
	".pi/skills",
	"~/.dsh/skills",
	"~/.agents/skills",
}

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
