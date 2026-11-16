package harness

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// NewAdapter constructs an adapter. Adapters are expected to be stateless:
// the registry constructs one only to read its descriptor, and Lookup
// constructs a fresh one per caller.
type NewAdapter func() Adapter

// ErrUnknownHarness is returned when an identifier is not registered.
var ErrUnknownHarness = errors.New("unknown harness")

// UnknownHarnessError names the missing identifier and unwraps to
// ErrUnknownHarness so callers can use errors.Is.
type UnknownHarnessError struct {
	ID string
}

func (e *UnknownHarnessError) Error() string {
	return fmt.Sprintf("unknown harness %q", e.ID)
}

func (e *UnknownHarnessError) Unwrap() error {
	return ErrUnknownHarness
}

// Registry maps stable harness identifiers to adapter constructors.
type Registry struct {
	mu          sync.RWMutex
	factories   map[string]NewAdapter
	descriptors []Descriptor
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]NewAdapter)}
}

// Register adds an adapter constructor under its descriptor identifier.
//
// The constructor is called once here to read the descriptor. Registering a
// nil constructor, an adapter with an empty identifier, or one whose
// identifier is already taken is an error: two adapters cannot answer to the
// same name.
func (r *Registry) Register(newAdapter NewAdapter) error {
	if newAdapter == nil {
		return errors.New("harness: nil adapter constructor")
	}

	adapter := newAdapter()
	if adapter == nil {
		return errors.New("harness: adapter constructor returned nil")
	}

	descriptor := adapter.Descriptor()
	if descriptor.ID == "" {
		return errors.New("harness: adapter descriptor has an empty id")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[descriptor.ID]; exists {
		return fmt.Errorf("harness: adapter %q is already registered", descriptor.ID)
	}

	r.factories[descriptor.ID] = newAdapter
	r.descriptors = append(r.descriptors, descriptor)
	return nil
}

// Lookup returns a fresh adapter for the identifier.
func (r *Registry) Lookup(id string) (Adapter, error) {
	r.mu.RLock()
	newAdapter, ok := r.factories[id]
	r.mu.RUnlock()

	if !ok {
		return nil, &UnknownHarnessError{ID: id}
	}
	return newAdapter(), nil
}

// All returns a fresh adapter per registered identifier, in registration
// order.
func (r *Registry) All() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapters := make([]Adapter, 0, len(r.factories))
	for _, descriptor := range r.descriptors {
		adapters = append(adapters, r.factories[descriptor.ID]())
	}
	return adapters
}

// Descriptors returns the registered descriptors in registration order.
func (r *Registry) Descriptors() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return append([]Descriptor(nil), r.descriptors...)
}

// Needs is what a workflow node requires from a harness.
//
// It is derived from node configuration, not from the workflow's graph, so a
// node can be validated against a harness without executing anything.
type Needs struct {
	NodeID      string
	Interactive bool
	Inject      bool
	Skill       string
	Model       string
	Tools       []string
}

// Validate reports what a harness cannot provide for a node, or nil when it
// can provide everything.
//
// It runs before execution on purpose: discovering an unmet requirement at
// node 5 of 8 wastes a run. Every miss is reported together so the user can
// fix the configuration in one pass.
func Validate(needs Needs, caps Capabilities) *Failure {
	var missing []string

	if needs.Interactive && !caps.Questions {
		missing = append(missing, "interactive questions (capability \"questions\")")
	}
	if needs.Inject && !caps.Injection {
		missing = append(missing, "mid-run injection (capability \"injection\")")
	}
	if needs.Skill != "" && !caps.Skills {
		missing = append(missing, fmt.Sprintf("skill %q (capability \"skills\")", needs.Skill))
	}
	if needs.Model != "" && !caps.ModelSelect {
		missing = append(missing, fmt.Sprintf("model %q (capability \"modelSelect\")", needs.Model))
	}
	if len(needs.Tools) > 0 && !caps.ToolPolicy {
		missing = append(missing, "tool selection (capability \"toolPolicy\")")
	}

	if len(missing) == 0 {
		return nil
	}

	return &Failure{
		Class:   FailureUnsupported,
		Message: fmt.Sprintf("node %q requires %s", needs.NodeID, strings.Join(missing, ", ")),
	}
}
