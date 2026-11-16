package openspec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrSkillNotFound reports a skill name that no configured root provides.
var ErrSkillNotFound = errors.New("skill not found")

// ResolveSkill maps a skill name to its absolute directory.
//
// Only the first match is returned: a harness loads a skill by path, and
// handing it two directories for one name would load the bundle twice.
func ResolveSkill(name string, roots []string) (string, error) {
	if name == "" {
		return "", nil
	}

	searched := make([]string, 0, len(roots))
	for _, root := range roots {
		candidate := filepath.Join(root, name)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
		searched = append(searched, candidate)
	}

	return "", fmt.Errorf("%w: %q (searched %s)", ErrSkillNotFound, name, strings.Join(searched, ", "))
}

// SkillRoot is one search root with a human-readable source label. The label is
// the configured root string, so the UI can group skills without exposing an
// absolute path.
type SkillRoot struct {
	Label string
	Path  string
}

// SkillInfo is one installed skill as the catalog presents it. Name is the
// directory name a workflow binds; the frontmatter description is advisory.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// ListSkills lists the skill directories that contain a SKILL.md, in root
// precedence order. A name supplied by a higher root shadows lower copies, so
// the catalog matches ResolveSkill. Unreadable roots and entries are skipped:
// a broken install must not hide the rest of the catalog.
func ListSkills(roots []SkillRoot) []SkillInfo {
	skills := make([]SkillInfo, 0)
	seen := make(map[string]bool)

	for _, root := range roots {
		entries, err := os.ReadDir(root.Path)
		if err != nil {
			continue
		}

		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			name := entry.Name()
			if !entry.IsDir() || seen[name] {
				continue
			}
			manifest := filepath.Join(root.Path, name, "SKILL.md")
			if info, err := os.Stat(manifest); err != nil || info.IsDir() {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}

		sort.Strings(names)
		for _, name := range names {
			skills = append(skills, SkillInfo{
				Name:        name,
				Description: skillDescription(filepath.Join(root.Path, name, "SKILL.md")),
				Source:      root.Label,
			})
		}
	}

	return skills
}

// skillDescription reads the frontmatter description without requiring a full
// skill parser. Any malformed or absent frontmatter simply leaves it empty.
func skillDescription(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return ""
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end == -1 {
		return ""
	}

	var meta struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Description)
}
