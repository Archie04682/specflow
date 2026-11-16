package harness

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubAdapter struct {
	descriptor   Descriptor
	capabilities Capabilities
}

func (s *stubAdapter) Descriptor() Descriptor { return s.descriptor }

func (s *stubAdapter) Capabilities() Capabilities { return s.capabilities }

func (s *stubAdapter) Probe(context.Context) (ProbeResult, error) {
	return ProbeResult{Capabilities: s.capabilities}, nil
}

func (s *stubAdapter) Run(context.Context, Request, Host) (Result, error) {
	panic("Run must not be called by the registry")
}

func stubFactory(id string, capabilities Capabilities) NewAdapter {
	return func() Adapter {
		return &stubAdapter{
			descriptor:   Descriptor{ID: id, Name: strings.ToUpper(id), Version: "0.1.0", Protocol: ProtocolVersion},
			capabilities: capabilities,
		}
	}
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(stubFactory("pi", Capabilities{Questions: true})); err != nil {
		t.Fatalf("Register: %v", err)
	}

	adapter, err := registry.Lookup("pi")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got := adapter.Descriptor().ID; got != "pi" {
		t.Fatalf("descriptor id = %q, want pi", got)
	}
	if !adapter.Capabilities().Questions {
		t.Fatal("capabilities lost the questions flag")
	}

	descriptors := registry.Descriptors()
	if len(descriptors) != 1 || descriptors[0].ID != "pi" {
		t.Fatalf("descriptors = %#v, want one entry for pi", descriptors)
	}
}

func TestRegistryRejectsDuplicateID(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(stubFactory("pi", Capabilities{})); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	err := registry.Register(stubFactory("pi", Capabilities{}))
	if err == nil {
		t.Fatal("Register with a duplicate id succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("error = %q, want it to name the duplicate", err)
	}
}

func TestRegistryRejectsInvalidFactories(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(nil); err == nil {
		t.Fatal("Register(nil) succeeded, want an error")
	}
	if err := registry.Register(func() Adapter { return nil }); err == nil {
		t.Fatal("Register with a nil adapter succeeded, want an error")
	}
	if err := registry.Register(stubFactory("", Capabilities{})); err == nil {
		t.Fatal("Register with an empty id succeeded, want an error")
	}
}

func TestRegistryUnknownHarness(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Lookup("nope")
	if err == nil {
		t.Fatal("Lookup of an unknown id succeeded, want an error")
	}
	if !errors.Is(err, ErrUnknownHarness) {
		t.Fatalf("error = %v, want it to satisfy errors.Is(err, ErrUnknownHarness)", err)
	}

	var unknown *UnknownHarnessError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %T, want *UnknownHarnessError", err)
	}
	if unknown.ID != "nope" {
		t.Fatalf("unknown id = %q, want nope", unknown.ID)
	}
}

func TestRegistryListOrderIsStable(t *testing.T) {
	registry := NewRegistry()
	for _, id := range []string{"b", "a", "c"} {
		if err := registry.Register(stubFactory(id, Capabilities{})); err != nil {
			t.Fatalf("Register(%q): %v", id, err)
		}
	}

	want := []string{"b", "a", "c"}
	for attempt := 0; attempt < 3; attempt++ {
		adapters := registry.All()
		if len(adapters) != len(want) {
			t.Fatalf("All() returned %d adapters, want %d", len(adapters), len(want))
		}
		for i, id := range want {
			if got := adapters[i].Descriptor().ID; got != id {
				t.Fatalf("attempt %d: All()[%d] = %q, want %q", attempt, i, got, id)
			}
		}
	}
}

// TestRegistryDoesNotExecuteWork guards the preflight contract: listing and
// describing harnesses must be free of side effects, because the API calls it
// on every request.
func TestRegistryDoesNotExecuteWork(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(func() Adapter {
		return &stubAdapter{
			descriptor:   Descriptor{ID: "pi", Name: "Pi", Version: "0.1.0", Protocol: ProtocolVersion},
			capabilities: Capabilities{Questions: true},
		}
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	registry.Descriptors()
	for _, adapter := range registry.All() {
		adapter.Descriptor()
		adapter.Capabilities()
		if _, err := adapter.Probe(context.Background()); err != nil {
			t.Fatalf("Probe: %v", err)
		}
	}
}

func TestValidate(t *testing.T) {
	all := Capabilities{
		Questions: true, Injection: true, Skills: true, ModelSelect: true, ToolPolicy: true,
	}

	cases := []struct {
		name      string
		needs     Needs
		caps      Capabilities
		wantNil   bool
		wantInMsg []string
	}{
		{
			name:    "satisfied",
			needs:   Needs{NodeID: "apply", Interactive: true, Inject: true, Skill: "openspec-apply-change", Model: "m", Tools: []string{"read"}},
			caps:    all,
			wantNil: true,
		},
		{
			name:      "interactive",
			needs:     Needs{NodeID: "explore", Interactive: true},
			caps:      Capabilities{Injection: true},
			wantInMsg: []string{"explore", "questions"},
		},
		{
			name:      "injection",
			needs:     Needs{NodeID: "apply", Inject: true},
			caps:      Capabilities{Questions: true},
			wantInMsg: []string{"apply", "injection"},
		},
		{
			name:      "skill",
			needs:     Needs{NodeID: "propose", Skill: "openspec-propose"},
			caps:      Capabilities{},
			wantInMsg: []string{"propose", "skills", "openspec-propose"},
		},
		{
			name:      "model",
			needs:     Needs{NodeID: "review", Model: "provider/model"},
			caps:      Capabilities{},
			wantInMsg: []string{"review", "modelSelect", "provider/model"},
		},
		{
			name:      "tools",
			needs:     Needs{NodeID: "tests", Tools: []string{"bash"}},
			caps:      Capabilities{},
			wantInMsg: []string{"tests", "toolPolicy"},
		},
		{
			name:      "multiple misses are reported together",
			needs:     Needs{NodeID: "explore", Interactive: true, Inject: true},
			caps:      Capabilities{},
			wantInMsg: []string{"questions", "injection"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failure := Validate(tc.needs, tc.caps)

			if tc.wantNil {
				if failure != nil {
					t.Fatalf("Validate = %v, want nil", failure)
				}
				return
			}

			if failure == nil {
				t.Fatal("Validate = nil, want a failure")
			}
			if failure.Class != FailureUnsupported {
				t.Fatalf("class = %q, want %q", failure.Class, FailureUnsupported)
			}
			for _, fragment := range tc.wantInMsg {
				if !strings.Contains(failure.Message, fragment) {
					t.Fatalf("message %q does not mention %q", failure.Message, fragment)
				}
			}
		})
	}
}
