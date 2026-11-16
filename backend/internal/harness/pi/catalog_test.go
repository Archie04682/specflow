package pi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeAgentFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestCatalogReadsModelsAndThinkingLevels(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", dir)
	writeAgentFile(t, dir, "settings.json", `{
		"enabledModels": ["deepseek/deepseek-flash"],
		"defaultProvider": "deepseek",
		"defaultModel": "deepseek-flash",
		"defaultTools": ["read", "custom_tool"]
	}`)
	writeAgentFile(t, dir, "models-store.json", `{
		"deepseek": {
			"models": [
				{
					"id": "deepseek-flash",
					"name": "DeepSeek V4.1 Flash",
					"reasoning": true,
					"thinkingLevelMap": {"minimal": null, "low": "low", "medium": null, "high": "high", "max": "max"}
				},
				{"id": "deepseek-chat", "name": "DeepSeek Chat", "reasoning": false}
			]
		},
		"other": {"models": [{"id": "other-model", "name": "Other"}]}
	}`)

	catalog, err := New(Options{}).Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}

	if len(catalog.Models) != 1 {
		t.Fatalf("models = %#v, want only the enabled model", catalog.Models)
	}
	model := catalog.Models[0]
	if model.ID != "deepseek/deepseek-flash" || model.Provider != "deepseek" {
		t.Fatalf("model = %#v", model)
	}
	if !model.Reasoning {
		t.Fatal("reasoning = false, want true")
	}
	wantLevels := []string{"off", "low", "high", "max"}
	if len(model.ThinkingLevels) != len(wantLevels) {
		t.Fatalf("thinking levels = %#v, want %#v", model.ThinkingLevels, wantLevels)
	}
	for index, level := range wantLevels {
		if model.ThinkingLevels[index] != level {
			t.Fatalf("thinking levels = %#v, want %#v", model.ThinkingLevels, wantLevels)
		}
	}
	if len(catalog.ThinkingLevels) == 0 {
		t.Fatal("harness thinking levels are empty")
	}

	toolNames := make(map[string]bool)
	for _, tool := range catalog.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"read", "write", "edit", "bash", "custom_tool"} {
		if !toolNames[name] {
			t.Fatalf("tool %q missing from %#v", name, catalog.Tools)
		}
	}
	if toolNames[askToolName] {
		t.Fatalf("catalog exposes the internal ask tool")
	}
}

func TestCatalogFallsBackWithoutConfig(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	catalog, err := New(Options{}).Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(catalog.Models) != 0 {
		t.Fatalf("models = %#v, want none", catalog.Models)
	}
	if len(catalog.Tools) == 0 || len(catalog.ThinkingLevels) == 0 {
		t.Fatalf("catalog = %#v, want tools and thinking levels", catalog)
	}
}

func TestCatalogUsesDefaultModelWhenStoreIsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", dir)
	writeAgentFile(t, dir, "settings.json", `{
		"enabledModels": [],
		"defaultProvider": "deepseek",
		"defaultModel": "deepseek-flash"
	}`)

	catalog, err := New(Options{}).Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(catalog.Models) != 1 || catalog.Models[0].ID != "deepseek/deepseek-flash" {
		t.Fatalf("models = %#v, want the default model as a fallback", catalog.Models)
	}
}
