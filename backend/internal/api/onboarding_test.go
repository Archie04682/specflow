package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/mock"
	"example.com/specflow/backend/internal/onboarding"
	"example.com/specflow/backend/internal/orchestrator"
)

// newOnboardingServer serves the API over a fresh git project with the given
// provisioning settings.
func newOnboardingServer(t *testing.T, options onboarding.Options) (*httptest.Server, string) {
	t.Helper()

	project := newRepo(t)
	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter { return mock.New(mock.Script{}) }); err != nil {
		t.Fatal(err)
	}
	engine := orchestrator.New(registry)
	t.Cleanup(engine.Close)

	server := httptest.NewServer(NewRouterWithOnboarding(fstestMapFS(), registry, engine, ServerIdentity{}, options))
	t.Cleanup(server.Close)
	return server, project
}

func decodeReadiness(t *testing.T, body []byte) onboarding.Readiness {
	t.Helper()

	var readiness onboarding.Readiness
	if err := json.Unmarshal(body, &readiness); err != nil {
		t.Fatalf("decode readiness: %v (%s)", err, body)
	}
	return readiness
}

func TestProjectReadinessReportsAFreshProject(t *testing.T) {
	server, project := newOnboardingServer(t, onboarding.Options{})

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/project/readiness", project), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", response.StatusCode, body)
	}
	readiness := decodeReadiness(t, body)
	if !readiness.Git.Repository || readiness.Git.Branch != "main" {
		t.Fatalf("git = %+v", readiness.Git)
	}
	if readiness.Ready {
		t.Fatal("a project with no flow was reported ready")
	}
}

func TestProjectReadinessTreatsANonRepositoryAsAGap(t *testing.T) {
	server, _ := newOnboardingServer(t, onboarding.Options{})
	directory := t.TempDir()

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/project/readiness", directory), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", response.StatusCode, body)
	}
	if decodeReadiness(t, body).Git.Repository {
		t.Fatal("a plain directory was reported as a repository")
	}
}

func TestProjectReadinessRefusesAMissingPath(t *testing.T) {
	server, _ := newOnboardingServer(t, onboarding.Options{})
	missing := filepath.Join(t.TempDir(), "missing")

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/project/readiness", missing), nil)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "missing") {
		t.Fatalf("body = %s", body)
	}
}

func TestProjectSeedFlowEndpoint(t *testing.T) {
	server, project := newOnboardingServer(t, onboarding.Options{})

	response, body := request(t, http.MethodPost, projectURL(server.URL, "/api/project/flow", project), map[string]string{"project": project})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("seed: %d %s", response.StatusCode, body)
	}
	var seeded struct {
		Reference string `json:"reference"`
		Path      string `json:"path"`
	}
	if err := json.Unmarshal(body, &seeded); err != nil {
		t.Fatal(err)
	}
	if seeded.Reference != onboarding.DefaultFlowReference {
		t.Fatalf("reference = %q", seeded.Reference)
	}
	if _, err := os.Stat(seeded.Path); err != nil {
		t.Fatalf("seeded flow: %v", err)
	}

	response, body = request(t, http.MethodPost, projectURL(server.URL, "/api/project/flow", project), map[string]string{"project": project})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("second seed: %d %s", response.StatusCode, body)
	}

	response, body = request(t, http.MethodPost, projectURL(server.URL, "/api/project/flow", project), map[string]string{"project": project, "reference": "no-such-flow"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown template: %d %s", response.StatusCode, body)
	}
}

func TestProjectInstallSkillsEndpoint(t *testing.T) {
	source := t.TempDir()
	writeSkillDir(t, source, "alpha", "Alpha")
	server, project := newOnboardingServer(t, onboarding.Options{SkillsSource: source})

	response, body := request(t, http.MethodPost, projectURL(server.URL, "/api/project/skills", project), map[string]any{"project": project})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("install: %d %s", response.StatusCode, body)
	}
	var report onboarding.InstallReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Installed) != 1 || report.Installed[0] != "alpha" {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatalf("installed skill: %v", err)
	}
}

func TestProjectInstallSkillsRefusesInOfflineMode(t *testing.T) {
	server, project := newOnboardingServer(t, onboarding.Options{Offline: true})

	response, body := request(t, http.MethodPost, projectURL(server.URL, "/api/project/skills", project), map[string]any{"project": project})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "offline") {
		t.Fatalf("body = %s", body)
	}
}

// writeSkillDir writes one skill into a source directory.
func writeSkillDir(t *testing.T, root, name, description string) {
	t.Helper()

	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n"
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowTemplatesEndpoint(t *testing.T) {
	server, _ := newOnboardingServer(t, onboarding.Options{})

	response, body := request(t, http.MethodGet, server.URL+"/api/templates", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", response.StatusCode, body)
	}
	var listing struct {
		Templates []string `json:"templates"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(listing.Templates, onboarding.DefaultFlowReference) {
		t.Fatalf("templates = %v", listing.Templates)
	}
}

func TestProjectInitRepositoryEndpoint(t *testing.T) {
	server, _ := newOnboardingServer(t, onboarding.Options{})
	directory := t.TempDir()

	response, body := request(t, http.MethodPost, projectURL(server.URL, "/api/project/repository", directory), map[string]string{"project": directory})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("init: %d %s", response.StatusCode, body)
	}
	var result struct {
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Branch != "main" {
		t.Fatalf("branch = %q", result.Branch)
	}
	if _, err := os.Stat(filepath.Join(directory, ".git")); err != nil {
		t.Fatalf(".git was not created: %v", err)
	}

	response, body = request(t, http.MethodPost, projectURL(server.URL, "/api/project/repository", directory), map[string]string{"project": directory})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("second init: %d %s", response.StatusCode, body)
	}
}

func TestProjectRepositoryStatusEndpoint(t *testing.T) {
	server, _ := newOnboardingServer(t, onboarding.Options{})
	directory := t.TempDir()

	response, body := request(t, http.MethodGet, projectURL(server.URL, "/api/project/repository", directory), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read: %d %s", response.StatusCode, body)
	}
	var status struct {
		Repository bool   `json:"repository"`
		Reason     string `json:"reason"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatal(err)
	}
	if status.Repository || status.Reason == "" {
		t.Fatalf("status = %+v", status)
	}

	if response, body := request(t, http.MethodPost, projectURL(server.URL, "/api/project/repository", directory), map[string]string{"project": directory}); response.StatusCode != http.StatusOK {
		t.Fatalf("init: %d %s", response.StatusCode, body)
	}
	response, body = request(t, http.MethodGet, projectURL(server.URL, "/api/project/repository", directory), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("re-read: %d %s", response.StatusCode, body)
	}
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatal(err)
	}
	if !status.Repository {
		t.Fatalf("status = %+v", status)
	}
}

func TestProjectSetupEndpoint(t *testing.T) {
	server, _ := newOnboardingServer(t, onboarding.Options{})

	folder := t.TempDir()
	response, body := request(t, http.MethodPost, server.URL+"/api/project/setup", map[string]string{
		"project": folder,
		"vcs":     "folder",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("setup folder: %d %s", response.StatusCode, body)
	}
	var result struct {
		VCS        string `json:"vcs"`
		Repository bool   `json:"repository"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.VCS != "folder" || result.Repository {
		t.Fatalf("result = %+v", result)
	}
	config, err := os.ReadFile(filepath.Join(folder, ".specflow", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "vcs: folder") {
		t.Fatalf("config = %s", config)
	}
	if _, err := os.Stat(filepath.Join(folder, ".git")); !os.IsNotExist(err) {
		t.Fatalf("folder mode created a repository: %v", err)
	}

	git := t.TempDir()
	response, body = request(t, http.MethodPost, server.URL+"/api/project/setup", map[string]string{
		"project": git,
		"vcs":     "git",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("setup git: %d %s", response.StatusCode, body)
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Repository {
		t.Fatalf("result = %+v", result)
	}

	response, body = request(t, http.MethodPost, server.URL+"/api/project/setup", map[string]string{
		"project": folder,
		"vcs":     "svn",
	})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown mode: %d %s", response.StatusCode, body)
	}
}

func TestProjectCommitEndpoint(t *testing.T) {
	server, _ := newOnboardingServer(t, onboarding.Options{})
	project := newRepo(t)
	if err := os.WriteFile(filepath.Join(project, "notes.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	response, body := request(t, http.MethodPost, server.URL+"/api/project/commit", map[string]string{
		"project": project,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("commit: %d %s", response.StatusCode, body)
	}
	var result struct {
		Commit string   `json:"commit"`
		Paths  []string `json:"paths"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Commit == "" || len(result.Paths) != 1 {
		t.Fatalf("result = %+v", result)
	}

	response, body = request(t, http.MethodPost, server.URL+"/api/project/commit", map[string]string{
		"project": project,
	})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("second commit: %d %s", response.StatusCode, body)
	}
}

func TestProjectCommitEndpointRefusesAndIgnoresSecrets(t *testing.T) {
	server, _ := newOnboardingServer(t, onboarding.Options{})
	project := newRepo(t)
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("TOKEN=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	response, body := request(t, http.MethodPost, server.URL+"/api/project/commit", map[string]string{
		"project": project,
	})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("commit: %d %s", response.StatusCode, body)
	}
	var refusal struct {
		Secrets []string `json:"secrets"`
	}
	if err := json.Unmarshal(body, &refusal); err != nil {
		t.Fatal(err)
	}
	if len(refusal.Secrets) != 1 || refusal.Secrets[0] != ".env" {
		t.Fatalf("refusal = %+v", refusal)
	}

	response, body = request(t, http.MethodPost, server.URL+"/api/project/ignore", map[string]any{
		"project": project,
		"paths":   refusal.Secrets,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ignore: %d %s", response.StatusCode, body)
	}

	response, body = request(t, http.MethodPost, server.URL+"/api/project/commit", map[string]string{
		"project": project,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("commit after ignore: %d %s", response.StatusCode, body)
	}
}
