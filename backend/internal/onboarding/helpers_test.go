package onboarding

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"example.com/specflow/backend/internal/harness"
)

// newGitProject creates a committed repository and returns its resolved root.
func newGitProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "initial")

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

// writeSkill creates one skill directory with a frontmatter description.
func writeSkill(t *testing.T, root, name, description string) {
	t.Helper()

	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// porcelain is the project's porcelain status.
func porcelain(t *testing.T, dir string) string {
	t.Helper()
	return gitRun(t, dir, "status", "--porcelain")
}

// testAdapter is a harness that always probes successfully, so readiness can be
// exercised without a real runtime.
type testAdapter struct{}

func (testAdapter) Descriptor() harness.Descriptor {
	return harness.Descriptor{ID: "pi", Name: "Pi", Version: "test", Protocol: harness.ProtocolVersion}
}

func (testAdapter) Capabilities() harness.Capabilities { return harness.Capabilities{} }

func (testAdapter) Probe(context.Context) (harness.ProbeResult, error) {
	return harness.ProbeResult{}, nil
}

func (testAdapter) Run(context.Context, harness.Request, harness.Host) (harness.Result, error) {
	return harness.Result{}, nil
}

// readyRegistry registers a harness the seeded template can use.
func readyRegistry(t *testing.T) *harness.Registry {
	t.Helper()

	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter { return testAdapter{} }); err != nil {
		t.Fatal(err)
	}
	return registry
}
