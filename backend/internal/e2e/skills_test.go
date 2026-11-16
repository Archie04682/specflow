package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/specflow/backend/internal/openspec"
	"gopkg.in/yaml.v3"
)

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// TestFixtureSkillsHaveFrontmatter keeps the fixtures loadable by Pi: a skill
// without a description is silently ignored, which is the exact failure the
// adapter's get_commands preflight exists to catch.
func TestFixtureSkillsHaveFrontmatter(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata", "skills"))
	if err != nil {
		t.Fatalf("read fixture skills: %v", err)
	}

	seen := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		seen++
		path := filepath.Join("testdata", "skills", entry.Name(), "SKILL.md")
		frontmatter := readFrontmatter(t, path)
		if frontmatter.Name != entry.Name() {
			t.Errorf("%s declares name %q, want %q", path, frontmatter.Name, entry.Name())
		}
		if strings.TrimSpace(frontmatter.Description) == "" {
			t.Errorf("%s has no description; Pi would not load it", path)
		}
	}
	if seen == 0 {
		t.Fatal("no fixture skills found")
	}
}

func TestFixtureSkillsResolveByDirectoryName(t *testing.T) {
	f := newFixture(t)

	for _, name := range []string{"write-file", "ask-and-record", "slow-bash", "create-change", "complete-tasks"} {
		path, err := openspec.ResolveSkill(name, []string{f.Skills})
		if err != nil {
			t.Fatalf("resolve %s: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
			t.Fatalf("%s did not resolve to a skill directory: %v", name, err)
		}
	}
}

func readFrontmatter(t *testing.T, path string) skillFrontmatter {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		t.Fatalf("%s does not start with YAML frontmatter", path)
	}
	header, _, found := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---")
	if !found {
		t.Fatalf("%s has unterminated frontmatter", path)
	}

	var frontmatter skillFrontmatter
	if err := yaml.Unmarshal([]byte(header), &frontmatter); err != nil {
		t.Fatalf("parse %s frontmatter: %v", path, err)
	}
	return frontmatter
}
