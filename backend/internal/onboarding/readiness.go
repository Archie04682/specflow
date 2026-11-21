// Package onboarding turns a registered folder into a project that can start a
// run: it reports what is missing, seeds the default flow from the executable,
// and installs the skills a flow binds from an online or an offline source.
package onboarding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/openspec"
	"example.com/specflow/backend/internal/orchestrator"
	"example.com/specflow/backend/internal/workspace"
)

// DefaultFlowReference is the flow a new project is offered first.
const DefaultFlowReference = "standard-feature"

// probeTimeout bounds the runtime probes a readiness read performs.
const probeTimeout = 3 * time.Second

// Errors a client distinguishes when a request is refused.
var (
	ErrProjectRequired = errors.New("project is required")
	ErrProjectNotDir   = errors.New("project path is not a directory")
)

// Options is the server's provisioning configuration plus the adapter registry
// readiness probes.
type Options struct {
	SkillsSource string
	Offline      bool
	Registry     *harness.Registry
}

// GitStatus is whether the project is a repository and what it has checked out.
// The workspace owns the read; HasCommit separates an initialised repository
// from one a worktree can be branched from.
type GitStatus = workspace.GitStatus

// FlowStatus is one workflow the project can run, with the skills it binds.
type FlowStatus struct {
	Reference      string   `json:"reference"`
	Path           string   `json:"path"`
	Valid          bool     `json:"valid"`
	Problems       []string `json:"problems"`
	RequiredSkills []string `json:"requiredSkills"`
	MissingSkills  []string `json:"missingSkills"`
}

// SkillsStatus is the project's skill roots and the skills it provides.
type SkillsStatus struct {
	Roots []string     `json:"roots"`
	Found []SkillEntry `json:"found"`
}

// SkillEntry is one installed skill as the project provides it.
type SkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// HarnessStatus is whether a harness a flow binds can run here.
type HarnessStatus struct {
	ID     string `json:"id"`
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

// ToolsStatus is the external tools the flows need.
type ToolsStatus struct {
	OpenSpecFound   bool            `json:"openspecFound"`
	OpenSpecVersion string          `json:"openspecVersion,omitempty"`
	Harnesses       []HarnessStatus `json:"harnesses"`
}

// Action is one provisioning step, offered only when it can actually run.
type Action struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Action names, in the order the card presents them.
const (
	ActionInitRepository      = "init_repository"
	ActionSeedFlow            = "seed_flow"
	ActionInstallSkillsLocal  = "install_skills_local"
	ActionInstallSkillsOnline = "install_skills_online"
)

// Readiness is everything a client needs to say what is missing.
type Readiness struct {
	Project       string       `json:"project"`
	VCS           string       `json:"vcs"`
	VCSExplicit   bool         `json:"vcsExplicit"`
	VCSChangeable bool         `json:"vcsChangeable"`
	Git           GitStatus    `json:"git"`
	Layout        bool         `json:"layout"`
	Flows         []FlowStatus `json:"flows"`
	Skills        SkillsStatus `json:"skills"`
	Tools         ToolsStatus  `json:"tools"`
	Uncommitted   []string     `json:"uncommitted"`
	Secrets       []string     `json:"secrets"`
	Offline       bool         `json:"offline"`
	SkillsSource  string       `json:"skillsSource,omitempty"`
	Actions       []Action     `json:"actions"`
	Ready         bool         `json:"ready"`
}

// Read reports a project's readiness without changing it.
func Read(ctx context.Context, projectPath string, options Options) (*Readiness, error) {
	root, err := projectDir(projectPath)
	if err != nil {
		return nil, err
	}

	// Every collection starts empty rather than nil: the JSON contract is an
	// array, and a null would make a client's first .filter call throw.
	readiness := &Readiness{
		Project:      root,
		Flows:        []FlowStatus{},
		Skills:       SkillsStatus{Roots: []string{}, Found: []SkillEntry{}},
		Tools:        ToolsStatus{Harnesses: []HarnessStatus{}},
		Uncommitted:  []string{},
		Secrets:      []string{},
		Offline:      options.Offline,
		SkillsSource: options.SkillsSource,
	}

	project, err := workspace.Open(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("open project: %w", err)
	}

	mode, err := project.Mode()
	if err != nil {
		return nil, err
	}
	readiness.VCS = mode
	explicit, err := project.ModeIsExplicit()
	if err != nil {
		return nil, err
	}
	readiness.VCSExplicit = explicit
	runs, err := project.HasRuns()
	if err != nil {
		return nil, err
	}
	readiness.VCSChangeable = !runs

	status, err := project.GitStatus(ctx)
	if err != nil {
		return nil, err
	}
	readiness.Git = status
	readiness.Layout = isDir(project.SpecflowDir())
	// A folder project is not measured against a clean checkout: the copy, not
	// the working tree, is the run's base.
	if mode == workspace.ModeGit {
		if paths, err := project.Uncommitted(ctx); err == nil && paths != nil {
			readiness.Uncommitted = paths
			readiness.Secrets = SecretPaths(paths)
		}
	}

	roots, err := project.SkillRoots()
	if err != nil {
		return nil, fmt.Errorf("read skill roots: %w", err)
	}
	readiness.Skills = skillsStatus(roots)

	flows, harnessIDs, err := flowStatuses(ctx, root, roots)
	if err != nil {
		return nil, err
	}
	readiness.Flows = flows
	readiness.Tools = toolsStatus(ctx, options.Registry, harnessIDs)
	// Only the default flow's own file decides whether seeding is still offered:
	// a project with some other flow can still want the standard one.
	defaultSeeded := isFile(filepath.Join(root, ".specflow", "workflows", DefaultFlowReference+".yaml"))
	readiness.Actions = actionsFor(options, mode, readiness.Git.Repository, defaultSeeded)
	readiness.Ready = isReady(readiness)
	return readiness, nil
}

// projectDir resolves a project path to an existing directory.
func projectDir(projectPath string) (string, error) {
	if strings.TrimSpace(projectPath) == "" {
		return "", ErrProjectRequired
	}
	absolute, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("resolve project %s: %w", projectPath, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("project %s: %w", projectPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrProjectNotDir, projectPath)
	}
	return absolute, nil
}

// skillsStatus lists the roots and the skills they currently provide.
func skillsStatus(roots []string) SkillsStatus {
	entries := make([]SkillEntry, 0)
	for _, skill := range openspec.ListSkills(labelsFor(roots)) {
		entries = append(entries, SkillEntry{Name: skill.Name, Description: skill.Description, Source: skill.Source})
	}
	return SkillsStatus{Roots: append([]string(nil), roots...), Found: entries}
}

// labelsFor pairs each root path with itself as the display label: the absolute
// path is already what the harness receives, so a synthetic label adds nothing.
func labelsFor(roots []string) []openspec.SkillRoot {
	labelled := make([]openspec.SkillRoot, 0, len(roots))
	for _, root := range roots {
		labelled = append(labelled, openspec.SkillRoot{Label: root, Path: root})
	}
	return labelled
}

// flowStatuses resolves each flow's bound skills and the harnesses it needs.
func flowStatuses(ctx context.Context, projectRoot string, roots []string) ([]FlowStatus, []string, error) {
	catalog, err := orchestrator.ReadWorkflowCatalog(ctx, projectRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("read workflow catalog: %w", err)
	}

	harnesses := make([]string, 0)
	seenHarness := make(map[string]bool)
	flows := make([]FlowStatus, 0, len(catalog))
	for _, summary := range catalog {
		status := FlowStatus{
			Reference:      summary.Reference,
			Path:           summary.Path,
			Valid:          summary.Valid,
			Problems:       append([]string{}, summary.Problems...),
			RequiredSkills: []string{},
			MissingSkills:  []string{},
		}

		required, bound := flowBindings(summary.Path)
		for _, id := range bound {
			if seenHarness[id] {
				continue
			}
			seenHarness[id] = true
			harnesses = append(harnesses, id)
		}
		for _, name := range required {
			status.RequiredSkills = append(status.RequiredSkills, name)
			if _, err := openspec.ResolveSkill(name, roots); err != nil {
				status.MissingSkills = append(status.MissingSkills, name)
			}
		}
		flows = append(flows, status)
	}
	sort.SliceStable(flows, func(i, j int) bool { return flows[i].Reference < flows[j].Reference })
	sort.Strings(harnesses)
	return flows, harnesses, nil
}

// flowBindings reads one definition for the skills and harnesses its nodes bind.
// A definition that cannot be read or parsed binds nothing; the flow still
// appears in the report through its catalog entry.
func flowBindings(path string) (skills []string, harnesses []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	workflow, err := orchestrator.ParseWorkflow(data, path)
	if err != nil {
		return nil, nil
	}

	required := make(map[string]bool)
	for index := range workflow.Nodes {
		node := &workflow.Nodes[index]
		for _, name := range node.EffectiveSkills() {
			required[name] = true
		}
		if id := workflow.EffectiveHarness(node); id != "" {
			harnesses = append(harnesses, id)
		}
	}
	return sortedKeys(required), harnesses
}

// toolsStatus probes the openspec CLI and every harness the flows bind.
func toolsStatus(ctx context.Context, registry *harness.Registry, harnessIDs []string) ToolsStatus {
	status := ToolsStatus{Harnesses: []HarnessStatus{}}

	if _, err := exec.LookPath("openspec"); err == nil {
		status.OpenSpecFound = true
		if version, err := openspecVersion(ctx); err == nil {
			status.OpenSpecVersion = version
		}
	}

	for _, id := range harnessIDs {
		entry := HarnessStatus{ID: id}
		if registry == nil {
			entry.Reason = "no harness registry"
			status.Harnesses = append(status.Harnesses, entry)
			continue
		}
		adapter, err := registry.Lookup(id)
		if err != nil {
			entry.Reason = reasonOf(err)
			status.Harnesses = append(status.Harnesses, entry)
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		_, probeErr := adapter.Probe(probeCtx)
		cancel()
		if probeErr != nil {
			entry.Reason = reasonOf(probeErr)
			status.Harnesses = append(status.Harnesses, entry)
			continue
		}
		entry.Ready = true
		status.Harnesses = append(status.Harnesses, entry)
	}
	return status
}

// openspecVersion reads the CLI version. Any failure is best-effort: the tool
// being present is the fact that matters.
func openspecVersion(ctx context.Context) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	output, err := exec.CommandContext(probeCtx, "openspec", "--version").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			return fields[len(fields)-1], nil
		}
	}
	return "", nil
}

// actionsFor reports which provisioning steps this server can actually run.
func actionsFor(options Options, mode string, repository bool, defaultSeeded bool) []Action {
	initialise := Action{Name: ActionInitRepository}
	switch {
	case mode == workspace.ModeFolder:
		initialise.Reason = "the project uses a folder workspace"
	case repository:
		initialise.Reason = "the project is already a git repository"
	default:
		initialise.Available = true
	}

	local := Action{Name: ActionInstallSkillsLocal}
	switch {
	case options.SkillsSource == "" && options.Offline:
		local.Reason = "offline mode: configure a skills source"
	case options.SkillsSource == "":
		local.Reason = "no skills source configured"
	case !pathExists(options.SkillsSource):
		local.Reason = "skills source not found: " + options.SkillsSource
	default:
		local.Available = true
	}

	online := Action{Name: ActionInstallSkillsOnline}
	switch {
	case options.Offline:
		online.Reason = "offline mode"
	case options.SkillsSource != "":
		online.Reason = "a local skills source is configured"
	default:
		online.Available = true
	}

	seed := Action{Name: ActionSeedFlow, Available: true}
	if template := templateFor(DefaultFlowReference, mode); !templateExists(template) {
		seed.Available = false
		seed.Reason = "this build carries no " + template + " template"
	} else if defaultSeeded {
		seed.Available = false
		seed.Reason = "the default flow already exists"
	}
	return []Action{initialise, seed, local, online}
}

// isReady reports whether the project can start a run as it stands.
func isReady(readiness *Readiness) bool {
	// A repository with no branch checked out cannot create a worktree or merge
	// a run back, so it is not ready even though it is a repository. A folder
	// project has no such requirement: its copy needs no base commit.
	if readiness.VCS != workspace.ModeFolder {
		if !readiness.Git.Repository || readiness.Git.Branch == "" || !readiness.Git.HasCommit {
			return false
		}
	}
	hasValidFlow := false
	for _, flow := range readiness.Flows {
		if !flow.Valid {
			continue
		}
		hasValidFlow = true
		if len(flow.MissingSkills) > 0 {
			return false
		}
	}
	if !hasValidFlow || len(readiness.Uncommitted) > 0 {
		return false
	}
	if !readiness.Tools.OpenSpecFound {
		return false
	}
	for _, harness := range readiness.Tools.Harnesses {
		if !harness.Ready {
			return false
		}
	}
	return true
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func reasonOf(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func sortedKeys[T any](set map[string]T) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
