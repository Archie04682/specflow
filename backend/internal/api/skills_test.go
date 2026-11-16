package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"example.com/specflow/backend/internal/openspec"
)

func configureSkillRoots(t *testing.T, project string, roots ...string) {
	t.Helper()
	body, err := json.Marshal(struct {
		Skills []string `yaml:"skills"`
	}{Skills: roots})
	if err != nil {
		t.Fatal(err)
	}
	// JSON is valid YAML; keep the test independent of a YAML encoder.
	if err := os.WriteFile(filepath.Join(project, ".specflow", "config.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCatalogSkill(t *testing.T, project, root, name, description string) {
	t.Helper()
	dir := filepath.Join(project, root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "---\nname: " + name + "\ndescription: \"" + description + "\"\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSkillCatalogEndpointListsProjectRootsInPrecedenceOrder(t *testing.T) {
	server, project := newRunServer(t)
	configureSkillRoots(t, project, ".specflow/skills", ".dsh/skills", "missing/skills")
	writeCatalogSkill(t, project, ".specflow/skills", "ui-ux-pro-max", "UI/UX design intelligence")
	writeCatalogSkill(t, project, ".specflow/skills", "alpha", "Alpha skill")
	writeCatalogSkill(t, project, ".dsh/skills", "ui-ux-pro-max", "Shadowed copy")
	writeCatalogSkill(t, project, ".dsh/skills", "openspec-explore", "Explore a change")

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/skills", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.StatusCode, body)
	}

	var payload struct {
		Skills []openspec.SkillInfo `json:"skills"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	want := []openspec.SkillInfo{
		{Name: "alpha", Description: "Alpha skill", Source: ".specflow/skills"},
		{Name: "ui-ux-pro-max", Description: "UI/UX design intelligence", Source: ".specflow/skills"},
		{Name: "openspec-explore", Description: "Explore a change", Source: ".dsh/skills"},
	}
	if len(payload.Skills) != len(want) {
		t.Fatalf("skills = %+v, want %+v", payload.Skills, want)
	}
	for index := range want {
		if payload.Skills[index] != want[index] {
			t.Fatalf("skills[%d] = %+v, want %+v", index, payload.Skills[index], want[index])
		}
	}
}

func TestSkillCatalogEndpointEmptyProjectReturnsAnArray(t *testing.T) {
	server, project := newRunServer(t)
	configureSkillRoots(t, project, ".specflow/skills")

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/skills", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.StatusCode, body)
	}

	var payload struct {
		Skills []openspec.SkillInfo `json:"skills"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Skills == nil || len(payload.Skills) != 0 {
		t.Fatalf("skills = %#v, want an empty array", payload.Skills)
	}
}

func TestSkillCatalogEndpointRejectsAnUnusableProject(t *testing.T) {
	server, _ := newRunServer(t)

	for _, target := range []string{
		server.URL + "/api/skills",
		projectURL(server.URL, "/api/skills", t.TempDir()),
	} {
		response, body := request(t, http.MethodGet, target, nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400: %s", target, response.StatusCode, body)
		}
	}
}
