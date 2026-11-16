package pi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"example.com/specflow/backend/internal/harness"
)

// piThinkingLevels mirrors the levels the Pi CLI documents. A model may narrow
// this set; "off" is always valid.
var piThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// piModel is the part of a Pi model definition the editor needs. Unknown
// fields are ignored so a newer Pi catalog does not break the listing.
type piModel struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Reasoning        bool               `json:"reasoning"`
	ThinkingLevelMap map[string]*string `json:"thinkingLevelMap"`
}

type piProviderModels struct {
	Models []piModel `json:"models"`
}

type piSettings struct {
	EnabledModels   []string `json:"enabledModels"`
	DefaultModel    string   `json:"defaultModel"`
	DefaultProvider string   `json:"defaultProvider"`
	DefaultTools    []string `json:"defaultTools"`
}

type piModelsJSON struct {
	Providers map[string]piProviderModels `json:"providers"`
}

// Catalog describes the models, thinking levels, and tools the local Pi
// installation understands. It reads the adapter's agent configuration; it
// never starts a runtime, so the editor can populate its pickers cheaply.
func (a *Adapter) Catalog(ctx context.Context) (harness.Catalog, error) {
	dir := os.Getenv("PI_CODING_AGENT_DIR")
	if strings.TrimSpace(dir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fallbackCatalog(nil), nil
		}
		dir = filepath.Join(home, ".pi", "agent")
	}

	settings := readPiSettings(filepath.Join(dir, "settings.json"))
	models := readModelStore(filepath.Join(dir, "models-store.json"))
	for provider, entry := range readModelsJSON(filepath.Join(dir, "models.json")) {
		for _, model := range entry.Models {
			models = append(models, namedPiModel{provider: provider, model: model})
		}
	}
	models = dedupeModels(models)

	catalog := fallbackCatalog(settings.DefaultTools)
	catalog.Models = buildModelOptions(models, settings)
	return catalog, nil
}

func fallbackCatalog(defaultTools []string) harness.Catalog {
	tools := make([]harness.ToolOption, 0, 8)
	for _, tool := range []harness.ToolOption{
		{Name: "read", Description: "Read files from the working directory."},
		{Name: "write", Description: "Create or overwrite a file."},
		{Name: "edit", Description: "Apply exact text replacements to a file."},
		{Name: "grep", Description: "Search file contents with a pattern."},
		{Name: "find", Description: "Find files by name or glob."},
		{Name: "ls", Description: "List directory contents."},
	} {
		tools = append(tools, tool)
	}
	if runtime.GOOS == "windows" {
		tools = append(tools, harness.ToolOption{Name: "powershell", Description: "Run a PowerShell command."})
	} else {
		tools = append(tools, harness.ToolOption{Name: "bash", Description: "Run a shell command."})
	}
	for _, name := range defaultTools {
		name = strings.TrimSpace(name)
		if name == "" || strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
			continue
		}
		found := false
		for _, tool := range tools {
			if tool.Name == name {
				found = true
				break
			}
		}
		if !found {
			tools = append(tools, harness.ToolOption{Name: name, Description: "Configured Pi tool."})
		}
	}
	return harness.Catalog{ThinkingLevels: append([]string(nil), piThinkingLevels...), Tools: tools}
}

type namedPiModel struct {
	provider string
	model    piModel
}

func readPiSettings(path string) piSettings {
	data, err := os.ReadFile(path)
	if err != nil {
		return piSettings{}
	}
	var settings piSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return piSettings{}
	}
	return settings
}

func readModelStore(path string) []namedPiModel {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var store map[string]piProviderModels
	if err := json.Unmarshal(data, &store); err != nil {
		return nil
	}
	models := make([]namedPiModel, 0)
	for provider, entry := range store {
		for _, model := range entry.Models {
			models = append(models, namedPiModel{provider: provider, model: model})
		}
	}
	return models
}

func readModelsJSON(path string) map[string]piProviderModels {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var document piModelsJSON
	if err := json.Unmarshal(data, &document); err != nil {
		return nil
	}
	return document.Providers
}

func dedupeModels(models []namedPiModel) []namedPiModel {
	seen := make(map[string]bool, len(models))
	result := make([]namedPiModel, 0, len(models))
	for _, model := range models {
		key := model.provider + "/" + model.model.ID
		if model.model.ID == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, model)
	}
	return result
}

func buildModelOptions(models []namedPiModel, settings piSettings) []harness.ModelOption {
	options := make([]harness.ModelOption, 0, len(models))
	for _, entry := range models {
		id := entry.model.ID
		if entry.provider != "" {
			id = entry.provider + "/" + id
		}
		if !modelEnabled(id, settings.EnabledModels) {
			continue
		}
		option := harness.ModelOption{
			ID:        id,
			Name:      entry.model.Name,
			Provider:  entry.provider,
			Reasoning: entry.model.Reasoning,
		}
		option.ThinkingLevels = modelThinkingLevels(entry.model)
		options = append(options, option)
	}
	if len(options) == 0 && settings.DefaultModel != "" {
		id := settings.DefaultModel
		if settings.DefaultProvider != "" && !strings.Contains(id, "/") {
			id = settings.DefaultProvider + "/" + id
		}
		options = append(options, harness.ModelOption{ID: id, Name: id, Provider: settings.DefaultProvider})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Provider != options[j].Provider {
			return options[i].Provider < options[j].Provider
		}
		return options[i].ID < options[j].ID
	})
	return options
}

func modelEnabled(id string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	modelID := id
	if slash := strings.Index(id, "/"); slash >= 0 {
		modelID = id[slash+1:]
	}
	for _, pattern := range patterns {
		if globMatch(pattern, id) || globMatch(pattern, modelID) {
			return true
		}
	}
	return false
}

func globMatch(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	quoted := regexp.QuoteMeta(pattern)
	quoted = strings.ReplaceAll(quoted, `\*`, `.*`)
	quoted = strings.ReplaceAll(quoted, `\?`, `.`)
	matched, err := regexp.MatchString("^"+quoted+"$", value)
	return err == nil && matched
}

func modelThinkingLevels(model piModel) []string {
	if !model.Reasoning {
		return []string{"off"}
	}
	if len(model.ThinkingLevelMap) == 0 {
		return append([]string(nil), piThinkingLevels...)
	}
	levels := []string{"off"}
	for _, level := range piThinkingLevels[1:] {
		if value, ok := model.ThinkingLevelMap[level]; ok && value != nil {
			levels = append(levels, level)
		}
	}
	return levels
}
