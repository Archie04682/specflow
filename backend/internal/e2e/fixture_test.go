package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"example.com/specflow/backend/internal/workspace"
)

// fixture is a small, clean git project the e2e ladder can run against. It is
// deliberately cheap to check out and has a real Makefile target, so the full
// workflow's verify node exercises something.
type fixture struct {
	Root   string
	Skills string
}

const fixtureGoMod = "module example.com/fixture\n\ngo 1.22\n"

const fixtureMain = `package main

// Greeting returns the fixture's greeting.
func Greeting() string { return "hello" }

func main() {}
`

const fixtureTest = `package main

import "testing"

func TestGreeting(t *testing.T) {
	if Greeting() != "hello" {
		t.Fatalf("Greeting() = %q", Greeting())
	}
}
`

const fixtureMakefile = "test:\n\tgo test ./...\n"

// newFixture creates the project, copies the fixture skills, writes the skill
// roots config, and commits everything. The run's worktree therefore starts
// from a clean tree and the project passes the engine's isolation check.
func newFixture(t *testing.T) *fixture {
	t.Helper()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), fixtureGoMod)
	writeFile(t, filepath.Join(root, "main.go"), fixtureMain)
	writeFile(t, filepath.Join(root, "main_test.go"), fixtureTest)
	writeFile(t, filepath.Join(root, "Makefile"), fixtureMakefile)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# Fixture\n\nA tiny project used by SpecFlow's e2e tests.\n")
	writeFile(t, filepath.Join(root, "openspec", "specs", ".keep"), "")
	writeFile(t, filepath.Join(root, "openspec", "changes", ".keep"), "")

	skills := filepath.Join(root, ".specflow", "skills")
	copyFixtureSkills(t, skills)
	writeFile(t, filepath.Join(root, ".specflow", "config.yaml"), "skills:\n  - .specflow/skills\n")

	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("-c", "user.name=SpecFlow Fixture", "-c", "user.email=fixture@example.com", "commit", "-q", "-m", "fixture")

	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{Root: resolved, Skills: filepath.Join(resolved, ".specflow", "skills")}
}

// writeWorkflow installs a workflow under the project's .specflow directory.
func (f *fixture) writeWorkflow(t *testing.T, name, body string) {
	t.Helper()
	writeFile(t, filepath.Join(f.Root, ".specflow", "workflows", name+".yaml"), body)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// copyFixtureSkills copies testdata/skills into the project so the compile
// step resolves them exactly as it would a user's skills.
func copyFixtureSkills(t *testing.T, destination string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join("testdata", "skills"))
	if err != nil {
		t.Fatalf("read fixture skills: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		source := filepath.Join("testdata", "skills", entry.Name(), "SKILL.md")
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", source, err)
		}
		writeFile(t, filepath.Join(destination, entry.Name(), "SKILL.md"), string(data))
	}
}

func TestFixtureProjectIsIsolatable(t *testing.T) {
	f := newFixture(t)

	project, err := workspace.Open(t.Context(), f.Root)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if err := project.CheckIsolatable(t.Context()); err != nil {
		t.Fatalf("fixture is not isolatable: %v", err)
	}

	for _, path := range []string{"go.mod", "main.go", "Makefile", "AGENTS.md", "openspec/changes", ".specflow/config.yaml"} {
		if _, err := os.Stat(filepath.Join(f.Root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("fixture is missing %s: %v", path, err)
		}
	}
}

func TestFixtureSkillsAreCopied(t *testing.T) {
	f := newFixture(t)

	entries, err := os.ReadDir(filepath.Join(f.Skills))
	if err != nil {
		t.Fatalf("read copied skills: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no skills were copied into the fixture")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(f.Skills, entry.Name(), "SKILL.md"))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if !strings.Contains(string(body), "name: "+entry.Name()) {
			t.Fatalf("skill %s does not declare its directory name", entry.Name())
		}
	}
}

// copyOpenSpecSkills copies the project's real OpenSpec skills into the
// fixture. The full workflow needs them; the cheap rungs deliberately do not.
func copyOpenSpecSkills(t *testing.T, root string) {
	t.Helper()

	source := filepath.Join("..", "..", "..", ".dsh", "skills")
	for _, name := range []string{
		"openspec-explore", "openspec-propose", "openspec-apply-change", "openspec-archive-change",
	} {
		copyTree(t, filepath.Join(source, name), filepath.Join(root, ".specflow", "skills", name))
	}
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()

	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s: %v", source, err)
	}
}
