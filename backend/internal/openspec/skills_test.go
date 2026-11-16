package openspec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSkillFindsTheFirstMatch(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	if err := os.MkdirAll(filepath.Join(second, "openspec-explore"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := ResolveSkill("openspec-explore", []string{first, second})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if path != filepath.Join(second, "openspec-explore") {
		t.Fatalf("path = %q", path)
	}
}

func TestResolveSkillReportsSearchedLocations(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	_, err := ResolveSkill("openspec-apply", []string{first, second})
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("err = %v, want ErrSkillNotFound", err)
	}

	message := err.Error()
	for _, root := range []string{first, second} {
		if !strings.Contains(message, filepath.Join(root, "openspec-apply")) {
			t.Fatalf("message %q does not list %q", message, root)
		}
	}
}

func TestResolveSkillEmptyNameIsNoSkill(t *testing.T) {
	path, err := ResolveSkill("", []string{t.TempDir()})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if path != "" {
		t.Fatalf("path = %q", path)
	}
}

func writeSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: \"" + description + "\"\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListSkillsReadsFrontmatterAndRespectsPrecedence(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	writeSkill(t, first, "beta", "Beta from the shared root")
	writeSkill(t, first, "alpha", "Alpha skill")
	writeSkill(t, second, "beta", "Shadowed beta")
	writeSkill(t, second, "gamma", "Gamma skill")
	if err := os.MkdirAll(filepath.Join(first, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	skills := ListSkills([]SkillRoot{
		{Label: ".specflow/skills", Path: first},
		{Label: ".dsh/skills", Path: second},
	})
	if len(skills) != 3 {
		t.Fatalf("skills = %+v, want three entries", skills)
	}

	want := []SkillInfo{
		{Name: "alpha", Description: "Alpha skill", Source: ".specflow/skills"},
		{Name: "beta", Description: "Beta from the shared root", Source: ".specflow/skills"},
		{Name: "gamma", Description: "Gamma skill", Source: ".dsh/skills"},
	}
	for index, entry := range want {
		if skills[index] != entry {
			t.Fatalf("skills[%d] = %+v, want %+v", index, skills[index], entry)
		}
	}
}

func TestListSkillsSkipsMissingRootsAndKeepsBrokenEntries(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "fine", "A usable skill")

	broken := filepath.Join(root, "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "SKILL.md"), []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := ListSkills([]SkillRoot{
		{Label: "missing", Path: filepath.Join(root, "does-not-exist")},
		{Label: "project", Path: root},
	})
	if len(skills) != 2 {
		t.Fatalf("skills = %+v, want two entries", skills)
	}
	if skills[0] != (SkillInfo{Name: "broken", Source: "project"}) {
		t.Fatalf("skills[0] = %+v", skills[0])
	}
	if skills[1] != (SkillInfo{Name: "fine", Description: "A usable skill", Source: "project"}) {
		t.Fatalf("skills[1] = %+v", skills[1])
	}
}

func TestListSkillsReturnsAnEmptySlice(t *testing.T) {
	skills := ListSkills(nil)
	if skills == nil || len(skills) != 0 {
		t.Fatalf("skills = %#v, want an empty non-nil slice", skills)
	}
}
