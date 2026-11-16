package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/mock"
)

type stubHarness struct {
	descriptor harness.Descriptor
	caps       harness.Capabilities
	probeErr   error
}

func (s stubHarness) Descriptor() harness.Descriptor { return s.descriptor }

func (s stubHarness) Capabilities() harness.Capabilities { return s.caps }

func (s stubHarness) Probe(context.Context) (harness.ProbeResult, error) {
	return harness.ProbeResult{Capabilities: s.caps}, s.probeErr
}

func (s stubHarness) Run(context.Context, harness.Request, harness.Host) (harness.Result, error) {
	panic("Run must not be called by the harness listing endpoint")
}

type harnessesBody struct {
	Harnesses []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Version    string `json:"version"`
		Protocol   int    `json:"protocol"`
		Ready      bool   `json:"ready"`
		ReadyError string `json:"readyError"`
		Caps       struct {
			Questions bool `json:"questions"`
		} `json:"capabilities"`
		Models []struct {
			ID             string   `json:"id"`
			Name           string   `json:"name"`
			Reasoning      bool     `json:"reasoning"`
			ThinkingLevels []string `json:"thinkingLevels"`
		} `json:"models"`
		ThinkingLevels []string `json:"thinkingLevels"`
		Tools          []struct {
			Name string `json:"name"`
		} `json:"tools"`
		CatalogError string `json:"catalogError"`
	} `json:"harnesses"`
}

type catalogHarness struct {
	stubHarness
	models []harness.ModelOption
	err    error
}

func (c catalogHarness) Catalog(context.Context) (harness.Catalog, error) {
	return harness.Catalog{
		Models:         c.models,
		ThinkingLevels: []string{"off", "high"},
		Tools:          []harness.ToolOption{{Name: "read"}, {Name: "bash"}},
	}, c.err
}

func decodeHarnesses(t *testing.T, rec *httptest.ResponseRecorder) harnessesBody {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body harnessesBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func TestHarnessesListsReadyHarness(t *testing.T) {
	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter {
		return mock.New(mock.Script{})
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	router := NewRouterWithHarnesses(fstest.MapFS{}, registry)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/harnesses", nil))

	body := decodeHarnesses(t, rec)
	if len(body.Harnesses) != 1 {
		t.Fatalf("harnesses = %d, want 1", len(body.Harnesses))
	}

	entry := body.Harnesses[0]
	if entry.ID != mock.DescriptorID {
		t.Fatalf("id = %q, want %q", entry.ID, mock.DescriptorID)
	}
	if !entry.Ready {
		t.Fatalf("ready = false, want true (readyError %q)", entry.ReadyError)
	}
	if entry.ReadyError != "" {
		t.Fatalf("readyError = %q, want empty", entry.ReadyError)
	}
	if !entry.Caps.Questions {
		t.Fatal("capabilities.questions = false, want true")
	}
	if entry.Protocol != harness.ProtocolVersion {
		t.Fatalf("protocol = %d, want %d", entry.Protocol, harness.ProtocolVersion)
	}
}

func TestHarnessesReportsUnavailableHarness(t *testing.T) {
	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter {
		return stubHarness{
			descriptor: harness.Descriptor{ID: "pi", Name: "Pi", Version: "0.1.0", Protocol: harness.ProtocolVersion},
			caps:       harness.Capabilities{Questions: true},
			probeErr: &harness.Failure{
				Class:   harness.FailureAdapterMissing,
				Message: "pi executable not found",
			},
		}
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	router := NewRouterWithHarnesses(fstest.MapFS{}, registry)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/harnesses", nil))

	body := decodeHarnesses(t, rec)
	if len(body.Harnesses) != 1 {
		t.Fatalf("harnesses = %d, want 1", len(body.Harnesses))
	}

	entry := body.Harnesses[0]
	if entry.Ready {
		t.Fatal("ready = true, want false for an unavailable harness")
	}
	if !strings.Contains(entry.ReadyError, "pi executable not found") {
		t.Fatalf("readyError = %q, want the probe failure named", entry.ReadyError)
	}
	if entry.Caps.Questions {
		t.Fatal("capabilities.questions = true for an unready harness; nothing works yet")
	}
}

func TestHarnessesIncludesOptionalCatalog(t *testing.T) {
	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter {
		return catalogHarness{
			stubHarness: stubHarness{
				descriptor: harness.Descriptor{ID: "pi", Name: "Pi", Version: "0.1.0", Protocol: harness.ProtocolVersion},
				caps:       harness.Capabilities{ModelSelect: true, ToolPolicy: true},
			},
			models: []harness.ModelOption{{
				ID:             "deepseek/deepseek-flash",
				Name:           "DeepSeek Flash",
				Reasoning:      true,
				ThinkingLevels: []string{"off", "high"},
			}},
		}
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	router := NewRouterWithHarnesses(fstest.MapFS{}, registry)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/harnesses", nil))

	body := decodeHarnesses(t, rec)
	entry := body.Harnesses[0]
	if len(entry.Models) != 1 || entry.Models[0].ID != "deepseek/deepseek-flash" {
		t.Fatalf("models = %#v", entry.Models)
	}
	if !entry.Models[0].Reasoning {
		t.Fatal("reasoning = false, want true")
	}
	if len(entry.ThinkingLevels) != 2 || len(entry.Tools) != 2 {
		t.Fatalf("thinking levels = %#v, tools = %#v", entry.ThinkingLevels, entry.Tools)
	}
	if entry.CatalogError != "" {
		t.Fatalf("catalogError = %q", entry.CatalogError)
	}
}

func TestHarnessesCatalogFailureDoesNotHideHarness(t *testing.T) {
	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter {
		return catalogHarness{
			stubHarness: stubHarness{
				descriptor: harness.Descriptor{ID: "pi", Name: "Pi", Version: "0.1.0", Protocol: harness.ProtocolVersion},
			},
			err: errors.New("catalog unreadable"),
		}
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	router := NewRouterWithHarnesses(fstest.MapFS{}, registry)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/harnesses", nil))

	body := decodeHarnesses(t, rec)
	if len(body.Harnesses) != 1 {
		t.Fatalf("harnesses = %d, want the entry to survive", len(body.Harnesses))
	}
	if !strings.Contains(body.Harnesses[0].CatalogError, "catalog unreadable") {
		t.Fatalf("catalogError = %q", body.Harnesses[0].CatalogError)
	}
}

func TestHarnessesUnknownSubpathIsJSONNotFound(t *testing.T) {
	router := NewRouterWithHarnesses(fstest.MapFS{}, harness.NewRegistry())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/harnesses/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestHarnessesEmptyRegistry(t *testing.T) {
	router := NewRouterWithHarnesses(fstest.MapFS{}, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/harnesses", nil))

	body := decodeHarnesses(t, rec)
	if len(body.Harnesses) != 0 {
		t.Fatalf("harnesses = %d, want none", len(body.Harnesses))
	}
	if !strings.Contains(rec.Body.String(), `"harnesses":[]`) {
		t.Fatalf("body = %s, want an empty JSON array rather than null", rec.Body.String())
	}
}
