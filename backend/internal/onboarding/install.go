package onboarding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"example.com/specflow/backend/internal/openspec"
	"example.com/specflow/backend/internal/orchestrator"
	"example.com/specflow/backend/internal/workspace"
)

// ErrOffline reports an install that needs the network while offline mode is
// active and no local source is configured.
var ErrOffline = errors.New("offline mode: configure a skills source")

// InstallReport is what an install did, per skill.
type InstallReport struct {
	Installed []string `json:"installed"`
	Unchanged []string `json:"unchanged"`
	Conflicts []string `json:"conflicts"`
	Missing   []string `json:"missing"`
	Output    string   `json:"output,omitempty"`
	ExitCode  int      `json:"exitCode,omitempty"`
}

// InstallSkills provisions a project's skills from the configured local source,
// or from the skills CLI when the network is allowed.
func InstallSkills(ctx context.Context, projectPath string, options Options, replace bool, runner CommandRunner) (InstallReport, error) {
	root, err := projectDir(projectPath)
	if err != nil {
		return InstallReport{}, err
	}

	var report InstallReport
	switch {
	case options.SkillsSource != "":
		report, err = installFromSource(root, options.SkillsSource, replace)
	case networkAllowed(options):
		report, err = installOnline(ctx, root, runner)
	default:
		return InstallReport{}, ErrOffline
	}
	if err != nil {
		return InstallReport{}, err
	}

	// The flow's needs are the yardstick: a skill the source skipped is missing
	// even when the source was otherwise complete.
	missing, err := missingSkills(ctx, root)
	if err != nil {
		return InstallReport{}, err
	}
	report.Missing = missing
	return report, nil
}

// installFromSource installs from a directory or archive. It needs no Node.js,
// no npm, and no network.
func installFromSource(projectRoot, sourcePath string, replace bool) (InstallReport, error) {
	source, err := openSkillsSource(sourcePath)
	if err != nil {
		return InstallReport{}, err
	}

	specflowDir := filepath.Join(projectRoot, ".specflow")
	if err := os.MkdirAll(specflowDir, 0o755); err != nil {
		return InstallReport{}, fmt.Errorf("create %s: %w", specflowDir, err)
	}
	scratch, err := os.MkdirTemp(specflowDir, "skills-")
	if err != nil {
		return InstallReport{}, fmt.Errorf("create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	if err := source.materialize(scratch); err != nil {
		return InstallReport{}, err
	}

	// The catalog's own rule decides what counts as a skill, so a directory
	// that would not appear in GET /api/skills is never reported as installed.
	available := make(map[string]bool)
	for _, skill := range openspec.ListSkills(labelsFor([]string{scratch})) {
		available[skill.Name] = true
	}

	report := InstallReport{
		Installed: []string{},
		Unchanged: []string{},
		Conflicts: []string{},
		Missing:   []string{},
	}
	targetRoot := filepath.Join(projectRoot, ".agents", "skills")
	for _, candidate := range source.candidates {
		if !available[candidate.name] {
			continue
		}
		staged := filepath.Join(scratch, candidate.name)
		target := filepath.Join(targetRoot, candidate.name)
		switch {
		case !pathExists(target):
			if err := copyTree(staged, target); err != nil {
				return InstallReport{}, fmt.Errorf("install %s: %w", candidate.name, err)
			}
			report.Installed = append(report.Installed, candidate.name)
		case treesEqual(staged, target):
			report.Unchanged = append(report.Unchanged, candidate.name)
		case replace:
			if err := os.RemoveAll(target); err != nil {
				return InstallReport{}, fmt.Errorf("replace %s: %w", candidate.name, err)
			}
			if err := copyTree(staged, target); err != nil {
				return InstallReport{}, fmt.Errorf("replace %s: %w", candidate.name, err)
			}
			report.Installed = append(report.Installed, candidate.name)
		default:
			report.Conflicts = append(report.Conflicts, candidate.name)
		}
	}
	return report, nil
}

// installOnline runs the skills CLI and reports its output and exit status. The
// report's per-skill lists come from comparing the installed tree before and
// after, because the CLI does not enumerate what it wrote.
func installOnline(ctx context.Context, projectRoot string, runner CommandRunner) (InstallReport, error) {
	if runner == nil {
		runner = ExecRunner
	}

	before := installedDigests(projectRoot)
	output, exitCode, runErr := runner(ctx, projectRoot,
		[]string{"DISABLE_TELEMETRY=1"},
		"npx", "-y", "skills", "add", SkillsRef,
		"--skill", "*", "--copy", "-a", "universal", "-y",
	)
	output = resultText(output, runErr)

	after := installedDigests(projectRoot)
	report := InstallReport{
		Installed: []string{},
		Unchanged: []string{},
		Conflicts: []string{},
		Missing:   []string{},
		Output:    output,
		ExitCode:  exitCode,
	}
	for name, digest := range after {
		if before[name] == digest {
			report.Unchanged = append(report.Unchanged, name)
			continue
		}
		report.Installed = append(report.Installed, name)
	}
	sort.Strings(report.Installed)
	sort.Strings(report.Unchanged)
	return report, nil
}

// resultText keeps a start failure visible: the CLI cannot report it itself.
func resultText(output string, err error) string {
	if err == nil {
		return output
	}
	if output == "" {
		return err.Error()
	}
	return output + "\n" + err.Error()
}

// installedDigests maps each installed skill name to a digest of its tree.
func installedDigests(projectRoot string) map[string]string {
	root := filepath.Join(projectRoot, ".agents", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		return map[string]string{}
	}
	digests := make(map[string]string, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		files, err := treeDigests(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		joined := ""
		for _, name := range sortedKeys(files) {
			joined += name + "=" + files[name] + ";"
		}
		digests[entry.Name()] = joined
	}
	return digests
}

// missingSkills is every skill the project's flows bind that resolves nowhere.
func missingSkills(ctx context.Context, projectRoot string) ([]string, error) {
	project, err := workspace.Open(ctx, projectRoot)
	if err != nil {
		return []string{}, nil
	}
	roots, err := project.SkillRoots()
	if err != nil {
		return nil, fmt.Errorf("read skill roots: %w", err)
	}
	catalog, err := orchestrator.ReadWorkflowCatalog(ctx, projectRoot)
	if err != nil {
		// A project whose catalog cannot be read has no flows to satisfy.
		return []string{}, nil
	}

	required := map[string]bool{}
	for _, summary := range catalog {
		data, err := os.ReadFile(summary.Path)
		if err != nil {
			continue
		}
		workflow, err := orchestrator.ParseWorkflow(data, summary.Path)
		if err != nil {
			continue
		}
		for index := range workflow.Nodes {
			for _, name := range workflow.Nodes[index].EffectiveSkills() {
				required[name] = true
			}
		}
	}

	missing := []string{}
	for _, name := range sortedKeys(required) {
		if _, err := openspec.ResolveSkill(name, roots); err != nil {
			missing = append(missing, name)
		}
	}
	return missing, nil
}
