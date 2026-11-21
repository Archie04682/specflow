package onboarding

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"example.com/specflow/backend/internal/orchestrator"
)

func installedSkill(t *testing.T, project, name string) string {
	t.Helper()

	path := filepath.Join(project, ".agents", "skills", name, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read installed %s: %v", name, err)
	}
	return string(data)
}

func TestInstallFromADirectoryInstallsEverySkill(t *testing.T) {
	project := newGitProject(t)
	source := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		writeSkill(t, source, name, name)
	}

	report, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Equal(report.Installed, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("installed = %v", report.Installed)
	}
	if len(report.Unchanged) != 0 || len(report.Conflicts) != 0 {
		t.Fatalf("report = %+v", report)
	}
	for _, name := range report.Installed {
		if installedSkill(t, project, name) == "" {
			t.Fatalf("%s was not written", name)
		}
	}
}

func TestReinstallingTheSameContentIsUnchanged(t *testing.T) {
	project := newGitProject(t)
	source := t.TempDir()
	writeSkill(t, source, "alpha", "Alpha")

	if _, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil); err != nil {
		t.Fatalf("first install: %v", err)
	}
	before := installedSkill(t, project, "alpha")

	report, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if len(report.Installed) != 0 || !slices.Equal(report.Unchanged, []string{"alpha"}) {
		t.Fatalf("report = %+v", report)
	}
	if after := installedSkill(t, project, "alpha"); after != before {
		t.Fatal("an unchanged install changed the bytes")
	}
}

func TestDifferentContentIsReportedNotReplaced(t *testing.T) {
	project := newGitProject(t)
	source := t.TempDir()
	writeSkill(t, source, "alpha", "Alpha")

	if _, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	installed := filepath.Join(project, ".agents", "skills", "alpha", "SKILL.md")
	writeFile(t, installed, "locally edited\n")

	report, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Equal(report.Conflicts, []string{"alpha"}) || len(report.Installed) != 0 {
		t.Fatalf("report = %+v", report)
	}
	if got := installedSkill(t, project, "alpha"); got != "locally edited\n" {
		t.Fatalf("conflict overwrote the skill: %q", got)
	}
}

func TestReplacementIsExplicit(t *testing.T) {
	project := newGitProject(t)
	source := t.TempDir()
	writeSkill(t, source, "alpha", "Alpha")

	if _, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	installed := filepath.Join(project, ".agents", "skills", "alpha", "SKILL.md")
	writeFile(t, installed, "locally edited\n")
	// A file the source does not carry must be gone after a replacement.
	writeFile(t, filepath.Join(project, ".agents", "skills", "alpha", "extra.md"), "extra\n")

	report, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, true, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Equal(report.Installed, []string{"alpha"}) {
		t.Fatalf("report = %+v", report)
	}
	if got := installedSkill(t, project, "alpha"); got == "locally edited\n" {
		t.Fatal("the skill was not replaced")
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "alpha", "extra.md")); !os.IsNotExist(err) {
		t.Fatal("replacement kept a file the source does not carry")
	}
}

func TestAPartialSourceLeavesTheRestMissing(t *testing.T) {
	project := newGitProject(t)
	if _, err := SeedFlow(project, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	source := t.TempDir()
	writeSkill(t, source, "openspec-explore", "Explore")

	report, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Equal(report.Installed, []string{"openspec-explore"}) {
		t.Fatalf("installed = %v", report.Installed)
	}
	want := []string{"openspec-apply-change", "openspec-propose"}
	if !slices.Equal(report.Missing, want) {
		t.Fatalf("missing = %v, want %v", report.Missing, want)
	}
}

// The installer and the catalog must agree on what a skill is, or readiness
// would claim an install the editor cannot see.
func TestInstalledSkillsAppearInTheCatalog(t *testing.T) {
	project := newGitProject(t)
	writeFile(t, filepath.Join(project, ".specflow", "config.yaml"), "skills:\n  - .agents/skills\n")

	source := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		writeSkill(t, source, name, name)
	}
	report, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	catalog, err := orchestrator.ReadSkillCatalog(context.Background(), project)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	names := make([]string, 0, len(catalog))
	for _, skill := range catalog {
		names = append(names, skill.Name)
	}
	if !slices.Equal(names, report.Installed) {
		t.Fatalf("catalog = %v, install reported %v", names, report.Installed)
	}
}

func TestInstallRefusesASourceWithNoSkills(t *testing.T) {
	project := newGitProject(t)
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "README.md"), "nothing\n")

	_, err := InstallSkills(context.Background(), project, Options{SkillsSource: source}, false, nil)
	if err == nil {
		t.Fatal("a source with no skills was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(project, ".agents")); !os.IsNotExist(statErr) {
		t.Fatal("a refused install wrote into the project")
	}
}
