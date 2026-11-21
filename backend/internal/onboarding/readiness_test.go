package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"example.com/specflow/backend/internal/workspace"
)

func TestReadReportsAFreshProject(t *testing.T) {
	project := newGitProject(t)

	readiness, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if readiness.Project != project {
		t.Fatalf("project = %q, want %q", readiness.Project, project)
	}
	if !readiness.Git.Repository || readiness.Git.Branch != "main" || !readiness.Git.HasCommit {
		t.Fatalf("git = %+v", readiness.Git)
	}
	if action := actionFor(readiness, ActionInitRepository); action.Available {
		t.Fatalf("init action = %+v, want it refused for a repository", action)
	}
	if readiness.Layout {
		t.Fatal("a fresh project reports a layout")
	}
	if len(readiness.Flows) != 0 || len(readiness.Uncommitted) != 0 {
		t.Fatalf("readiness = %+v", readiness)
	}
	if readiness.Ready {
		t.Fatal("a project with no flow was reported ready")
	}
	if actionFor(readiness, ActionSeedFlow).Available != true {
		t.Fatalf("seed action = %+v", actionFor(readiness, ActionSeedFlow))
	}
	if actionFor(readiness, ActionInstallSkillsLocal).Available {
		t.Fatal("a local install is offered with no source configured")
	}
	if !actionFor(readiness, ActionInstallSkillsOnline).Available {
		t.Fatal("the online install is not offered on an online server")
	}
}

func TestReadCreatesNothing(t *testing.T) {
	project := newGitProject(t)
	before := porcelain(t, project)

	if _, err := Read(context.Background(), project, Options{}); err != nil {
		t.Fatalf("first read: %v", err)
	}
	first, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("second read: %v", err)
	}

	if after := porcelain(t, project); after != before {
		t.Fatalf("readiness changed the tree:\nbefore: %s\nafter: %s", before, after)
	}
	if _, err := os.Stat(filepath.Join(project, ".specflow")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readiness created .specflow: %v", err)
	}
	second, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("third read: %v", err)
	}
	if !slices.Equal(first.Skills.Roots, second.Skills.Roots) || first.Ready != second.Ready {
		t.Fatal("two reads disagree")
	}
}

func TestReadTreatsANonRepositoryAsAGap(t *testing.T) {
	directory := t.TempDir()

	readiness, err := Read(context.Background(), directory, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if readiness.Git.Repository {
		t.Fatal("a plain directory was reported as a repository")
	}
	if readiness.Git.Reason == "" {
		t.Fatal("no reason was reported")
	}
	if readiness.Ready {
		t.Fatal("a non-repository was reported ready")
	}
}

func TestReadRefusesAMissingPath(t *testing.T) {
	_, err := Read(context.Background(), filepath.Join(t.TempDir(), "missing"), Options{})
	if err == nil {
		t.Fatal("a missing path was accepted")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %q", err)
	}
}

func TestReadDerivesRequiredSkillsFromTheFlows(t *testing.T) {
	project := newGitProject(t)
	if _, err := SeedFlow(project, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	readiness, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(readiness.Flows) != 1 {
		t.Fatalf("flows = %+v", readiness.Flows)
	}
	flow := readiness.Flows[0]
	want := []string{"openspec-apply-change", "openspec-explore", "openspec-propose"}
	if !slices.Equal(flow.RequiredSkills, want) {
		t.Fatalf("required = %v, want %v", flow.RequiredSkills, want)
	}
	if !slices.Equal(flow.MissingSkills, want) {
		t.Fatalf("missing = %v, want %v", flow.MissingSkills, want)
	}
	if len(readiness.Skills.Roots) != 6 {
		t.Fatalf("roots = %v", readiness.Skills.Roots)
	}

	// Two of the three are installed; the third stays missing.
	skills := filepath.Join(t.TempDir(), "skills")
	writeSkill(t, skills, "openspec-explore", "Explore")
	writeSkill(t, skills, "openspec-propose", "Propose")
	if _, err := InstallSkills(context.Background(), project, Options{SkillsSource: skills}, false, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	readiness, err = Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := readiness.Flows[0].MissingSkills; !slices.Equal(got, []string{"openspec-apply-change"}) {
		t.Fatalf("missing = %v", got)
	}
}

func TestReadListsABrokenFlowWithoutHidingTheOthers(t *testing.T) {
	project := newGitProject(t)
	writeFile(t, filepath.Join(project, ".specflow", "workflows", "broken.yaml"), "id: [unclosed\n")
	writeFile(t, filepath.Join(project, ".specflow", "workflows", "legacy.yaml"), `
id: legacy
defaults: {harness: fake}
nodes:
  - {id: single, kind: agent, prompt: work, skill: legacy-skill}
`)

	readiness, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(readiness.Flows) != 2 {
		t.Fatalf("flows = %+v", readiness.Flows)
	}

	byReference := map[string]FlowStatus{}
	for _, flow := range readiness.Flows {
		byReference[flow.Reference] = flow
	}
	broken := byReference["broken"]
	if broken.Valid || len(broken.Problems) == 0 {
		t.Fatalf("broken flow = %+v", broken)
	}
	legacy := byReference["legacy"]
	if !legacy.Valid {
		t.Fatalf("legacy flow = %+v", legacy)
	}
	// The older singular `skill` field is still a binding.
	if !slices.Equal(legacy.RequiredSkills, []string{"legacy-skill"}) {
		t.Fatalf("required = %v", legacy.RequiredSkills)
	}
}

func TestReadReportsUncommittedInstalledSkills(t *testing.T) {
	project := newGitProject(t)
	skills := filepath.Join(t.TempDir(), "skills")
	writeSkill(t, skills, "openspec-explore", "Explore")
	if _, err := InstallSkills(context.Background(), project, Options{SkillsSource: skills}, false, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	readiness, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !slices.Contains(readiness.Uncommitted, ".agents") {
		t.Fatalf("uncommitted = %v, want .agents", readiness.Uncommitted)
	}

	gitRun(t, project, "add", "-A")
	gitRun(t, project, "commit", "-q", "-m", "install skills")
	readiness, err = Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(readiness.Uncommitted) != 0 {
		t.Fatalf("uncommitted = %v after the commit", readiness.Uncommitted)
	}
}

func TestReadAProvisionedCommittedProjectIsReady(t *testing.T) {
	project := newGitProject(t)
	if _, err := SeedFlow(project, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A seeded flow lives under .specflow/, which the run gate ignores, so it
	// is not a run blocker.
	readiness, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(readiness.Uncommitted) != 0 {
		t.Fatalf("a seeded flow was reported as uncommitted: %v", readiness.Uncommitted)
	}

	skills := filepath.Join(t.TempDir(), "skills")
	for _, name := range []string{"openspec-explore", "openspec-propose", "openspec-apply-change"} {
		writeSkill(t, skills, name, name)
	}
	if _, err := InstallSkills(context.Background(), project, Options{SkillsSource: skills}, false, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	gitRun(t, project, "add", "-A")
	gitRun(t, project, "commit", "-q", "-m", "provision")

	readiness, err = Read(context.Background(), project, Options{Registry: readyRegistry(t)})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !readiness.Tools.OpenSpecFound {
		t.Skip("the openspec CLI is not on PATH in this environment")
	}
	if !readiness.Ready {
		t.Fatalf("provisioned project is not ready: %+v", readiness)
	}
}

func actionFor(readiness *Readiness, name string) Action {
	for _, action := range readiness.Actions {
		if action.Name == name {
			return action
		}
	}
	return Action{}
}

// A project with a flow of its own can still want the default one.
func TestOtherFlowsDoNotHideTheSeedAction(t *testing.T) {
	project := newGitProject(t)
	writeFile(t, filepath.Join(project, ".specflow", "workflows", "custom.yaml"), `
id: custom
defaults: {harness: fake}
nodes:
  - {id: only, kind: agent, prompt: work}
`)

	readiness, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(readiness.Flows) != 1 || readiness.Flows[0].Reference != "custom" {
		t.Fatalf("flows = %+v", readiness.Flows)
	}
	if action := actionFor(readiness, ActionSeedFlow); !action.Available {
		t.Fatalf("seed action = %+v, want it offered", action)
	}
}

func TestIsReadyRequiresAUsableRepository(t *testing.T) {
	provisioned := func() *Readiness {
		return &Readiness{
			Git:   GitStatus{Repository: true, Branch: "main", HasCommit: true},
			Flows: []FlowStatus{{Reference: "flow", Valid: true, MissingSkills: []string{}}},
			Tools: ToolsStatus{OpenSpecFound: true},
		}
	}

	if !isReady(provisioned()) {
		t.Fatal("a provisioned project is not ready")
	}

	detached := provisioned()
	detached.Git.Branch = ""
	if isReady(detached) {
		t.Fatal("a detached HEAD was reported ready")
	}

	noCommit := provisioned()
	noCommit.Git.HasCommit = false
	if isReady(noCommit) {
		t.Fatal("a repository with no commit was reported ready")
	}

	notRepository := provisioned()
	notRepository.Git.Repository = false
	if isReady(notRepository) {
		t.Fatal("a non-repository was reported ready")
	}

	dirty := provisioned()
	dirty.Uncommitted = []string{".agents"}
	if isReady(dirty) {
		t.Fatal("an uncommitted provisioning path was reported ready")
	}

	missing := provisioned()
	missing.Flows[0].MissingSkills = []string{"openspec-explore"}
	if isReady(missing) {
		t.Fatal("a missing skill was reported ready")
	}

	noFlow := provisioned()
	noFlow.Flows = nil
	if isReady(noFlow) {
		t.Fatal("a project with no valid flow was reported ready")
	}

	noTool := provisioned()
	noTool.Tools.OpenSpecFound = false
	if isReady(noTool) {
		t.Fatal("a project without the openspec CLI was reported ready")
	}

	harnessNotReady := provisioned()
	harnessNotReady.Tools.Harnesses = []HarnessStatus{{ID: "pi", Reason: "missing"}}
	if isReady(harnessNotReady) {
		t.Fatal("a project whose harness is not ready was reported ready")
	}
}

func TestReadReportsADetachedHead(t *testing.T) {
	project := newGitProject(t)
	gitRun(t, project, "checkout", "-q", "--detach")

	readiness, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !readiness.Git.Repository {
		t.Fatal("a detached repository was reported as not a repository")
	}
	if readiness.Git.Branch != "" {
		t.Fatalf("branch = %q, want none", readiness.Git.Branch)
	}
	if !strings.Contains(readiness.Git.Reason, "detached") {
		t.Fatalf("reason = %q", readiness.Git.Reason)
	}
	if readiness.Ready {
		t.Fatal("a detached HEAD was reported ready")
	}
}

func TestReadReportsAnInitialisedRepositoryWithNoCommit(t *testing.T) {
	directory := t.TempDir()
	writeFile(t, filepath.Join(directory, "README.md"), "hello\n")
	if _, err := InitRepository(context.Background(), directory); err != nil {
		t.Fatalf("init: %v", err)
	}

	readiness, err := Read(context.Background(), directory, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !readiness.Git.Repository {
		t.Fatal("the initialised directory is not reported as a repository")
	}
	if readiness.Git.Branch != "main" {
		t.Fatalf("branch = %q", readiness.Git.Branch)
	}
	if readiness.Git.HasCommit {
		t.Fatal("a repository with no commit reports one")
	}
	if readiness.Ready {
		t.Fatal("a repository with no commit was reported ready")
	}
	// The untracked file is still the next step, not a silent pass.
	if len(readiness.Uncommitted) == 0 {
		t.Fatal("the operator's files are not reported as uncommitted")
	}
}

// Empty collections are arrays on the wire, never null: the client is typed to
// arrays, and a null is a render crash rather than a missing value.
func TestReadMarshalsEmptyCollectionsAsArrays(t *testing.T) {
	directory := t.TempDir()

	readiness, err := Read(context.Background(), directory, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	encoded, err := json.Marshal(readiness)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, fragment := range []string{
		`"flows":[]`,
		`"harnesses":[]`,
		`"found":[]`,
		`"uncommitted":[]`,
		`"secrets":[]`,
		`"actions":[`,
	} {
		if !strings.Contains(string(encoded), fragment) {
			t.Fatalf("readiness has no %s:\n%s", fragment, encoded)
		}
	}
	if strings.Contains(string(encoded), "null") {
		t.Fatalf("readiness carries a null collection:\n%s", encoded)
	}
}

func TestIsReadyForAFolderProjectNeedsNoRepository(t *testing.T) {
	readiness := &Readiness{
		VCS:   workspace.ModeFolder,
		Git:   GitStatus{Repository: false, Reason: "not a git repository"},
		Flows: []FlowStatus{{Reference: "flow", Valid: true, MissingSkills: []string{}}},
		Tools: ToolsStatus{OpenSpecFound: true},
	}
	if !isReady(readiness) {
		t.Fatalf("a provisioned folder project is not ready: %+v", readiness)
	}
}

func TestReadReportsAFolderProjectWithoutAGitGap(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, ".specflow", "config.yaml"), "vcs: folder\n")
	writeFile(t, filepath.Join(project, ".specflow", "workflows", "one.yaml"), `
id: one
defaults: {harness: fake}
nodes:
  - {id: only, kind: agent, prompt: work}
`)

	readiness, err := Read(context.Background(), project, Options{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if readiness.VCS != workspace.ModeFolder || !readiness.VCSExplicit || !readiness.VCSChangeable {
		t.Fatalf("mode = %q, explicit = %v, changeable = %v", readiness.VCS, readiness.VCSExplicit, readiness.VCSChangeable)
	}
	if readiness.Git.Repository {
		t.Fatal("a folder project reported a repository")
	}
	if len(readiness.Uncommitted) != 0 {
		t.Fatalf("uncommitted = %v", readiness.Uncommitted)
	}
	if action := actionFor(readiness, ActionInitRepository); action.Available || action.Reason == "" {
		t.Fatalf("init action = %+v, want it unavailable with a reason", action)
	}
	if len(readiness.Flows) != 1 || readiness.Flows[0].Reference != "one" {
		t.Fatalf("flows = %+v", readiness.Flows)
	}
}
