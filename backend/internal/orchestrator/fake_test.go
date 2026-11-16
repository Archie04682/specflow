package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
)

type fakeState struct {
	mu       sync.Mutex
	calls    map[string]int
	order    []string
	requests []harness.Request
}

func newFakeState() *fakeState {
	return &fakeState{calls: make(map[string]int)}
}

func (s *fakeState) count(nodeID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[nodeID]
}

func (s *fakeState) request(nodeID string) (harness.Request, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, request := range s.requests {
		if request.NodeID == nodeID {
			return request, true
		}
	}
	return harness.Request{}, false
}

// fakeBehavior scripts one node's execution.
type fakeBehavior struct {
	Writes           map[string]string
	Ask              *harness.Ask
	WaitForInjection bool
	Events           int
	// Failures makes the first N calls fail.
	Failures    int
	Retryable   bool
	FailClass   harness.FailureClass
	FailMessage string
	// CheckAbsent fails when the named file already exists, which proves a
	// clean retry restored the workspace.
	CheckAbsent string
	Sleep       time.Duration
	Text        string
}

type fakeAdapter struct {
	state     *fakeState
	behaviors map[string]*fakeBehavior
	caps      harness.Capabilities
}

func (f *fakeAdapter) Descriptor() harness.Descriptor {
	return harness.Descriptor{ID: "fake", Name: "Fake", Version: "0.1.0", Protocol: harness.ProtocolVersion}
}

func (f *fakeAdapter) Capabilities() harness.Capabilities { return f.caps }

func (f *fakeAdapter) Probe(context.Context) (harness.ProbeResult, error) {
	return harness.ProbeResult{Capabilities: f.caps}, nil
}

func (f *fakeAdapter) Run(ctx context.Context, request harness.Request, host harness.Host) (harness.Result, error) {
	f.state.mu.Lock()
	f.state.calls[request.NodeID]++
	call := f.state.calls[request.NodeID]
	f.state.order = append(f.state.order, request.NodeID)
	f.state.requests = append(f.state.requests, request)
	f.state.mu.Unlock()

	behavior := f.behaviors[request.NodeID]
	if behavior == nil {
		behavior = &fakeBehavior{}
	}

	if behavior.Sleep > 0 {
		select {
		case <-time.After(behavior.Sleep):
		case <-ctx.Done():
			return contextFailure(ctx)
		}
	}

	if behavior.CheckAbsent != "" {
		if _, err := os.Stat(filepath.Join(request.Workdir, behavior.CheckAbsent)); err == nil {
			return fakeFailure(harness.FailureHarnessExit, behavior.CheckAbsent+" was present", false)
		}
	}

	for name, content := range behavior.Writes {
		path := filepath.Join(request.Workdir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fakeFailure(harness.FailureConfig, err.Error(), false)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fakeFailure(harness.FailureConfig, err.Error(), false)
		}
	}

	for index := 0; index < behavior.Events; index++ {
		event := harness.Event{Kind: harness.KindTextDelta, Text: "working"}
		if err := host.Emit(event); err != nil {
			return fakeFailure(harness.FailureProtocol, err.Error(), false)
		}
	}

	text := behavior.Text
	if behavior.Ask != nil {
		answer, err := host.Ask(ctx, *behavior.Ask)
		if err != nil {
			return fakeFailure(harness.FailureCancelled, err.Error(), false)
		}
		if answer.Cancelled {
			return contextFailure(ctx)
		}
		text = answer.Value
	}

	if behavior.WaitForInjection {
		select {
		case injection := <-host.Injections():
			if err := host.Emit(harness.Event{
				Kind:      harness.KindInjectionAck,
				Injection: &harness.InjectionAck{ID: injection.ID, Disposition: "delivered"},
			}); err != nil {
				return fakeFailure(harness.FailureProtocol, err.Error(), false)
			}
		case <-ctx.Done():
			return contextFailure(ctx)
		}
	}

	if call <= behavior.Failures {
		class := behavior.FailClass
		if class == "" {
			class = harness.FailureHarnessExit
		}
		message := behavior.FailMessage
		if message == "" {
			message = "scripted failure"
		}
		return fakeFailure(class, message, behavior.Retryable)
	}

	return harness.Result{Status: harness.StatusCompleted, Text: text}, nil
}

// contextFailure reports why a blocked execution stopped, mirroring an adapter
// that distinguishes its own deadline from a user cancellation.
func contextFailure(ctx context.Context) (harness.Result, error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result := harness.Result{
			Status:  harness.StatusTimedOut,
			Failure: &harness.Failure{Class: harness.FailureTimeout, Message: "timed out", Retryable: true},
		}
		return result, result.Err()
	}
	return fakeFailure(harness.FailureCancelled, "cancelled", false)
}

func fakeFailure(class harness.FailureClass, message string, retryable bool) (harness.Result, error) {
	result := harness.Result{
		Status:  harness.StatusFailed,
		Failure: &harness.Failure{Class: class, Message: message, Retryable: retryable},
	}
	return result, result.Err()
}

func allCapabilities() harness.Capabilities {
	return harness.Capabilities{
		Streaming: true, Thinking: true, ToolEvents: true, FileEvents: true,
		Questions: true, Injection: true, Abort: true, Skills: true,
		ModelSelect: true, ToolPolicy: true, Plugins: true, Usage: true,
	}
}

func fakeRegistry(t *testing.T, state *fakeState, behaviors map[string]*fakeBehavior, caps harness.Capabilities) *harness.Registry {
	t.Helper()

	registry := harness.NewRegistry()
	err := registry.Register(func() harness.Adapter {
		return &fakeAdapter{state: state, behaviors: behaviors, caps: caps}
	})
	if err != nil {
		t.Fatalf("register fake adapter: %v", err)
	}
	return registry
}

// newTestEngine returns an engine backed by the scripted fake harness with no
// per-node behavior overrides.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()

	engine := New(fakeRegistry(t, newFakeState(), nil, allCapabilities()))
	t.Cleanup(engine.Close)
	return engine
}

// newRepoProject creates a committed git project and returns its resolved root.
func newRepoProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	git := func(args ...string) {
		command := exec.Command("git", args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "initial")

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProjectWorkflow(t *testing.T, projectRoot, yamlText string) {
	t.Helper()

	writeTestFile(t, filepath.Join(projectRoot, ".specflow", "workflows", "test.yaml"), yamlText)
}

func startRun(t *testing.T, engine *Engine, project, objective string) *Run {
	t.Helper()

	ctx := context.Background()
	run, err := engine.Create(ctx, CreateRequest{Project: project, Workflow: "test", Objective: objective})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := engine.Start(ctx, project, run.ID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	return run
}

func waitFor(t *testing.T, engine *Engine, projectRoot, runID string, predicate func(*Run) bool) *Run {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		run, err := engine.Get(context.Background(), projectRoot, runID)
		if err == nil && predicate(run) {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}

	run, err := engine.Get(context.Background(), projectRoot, runID)
	if err != nil {
		t.Fatalf("timed out and could not load run: %v", err)
	}
	t.Fatalf("timed out; run status %s, nodes %+v", run.Status, run.Nodes)
	return nil
}

func waitTerminal(t *testing.T, engine *Engine, projectRoot, runID string) *Run {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		run, err := engine.Get(context.Background(), projectRoot, runID)
		if err == nil && run.Status.Terminal() {
			engine.mu.Lock()
			_, live := engine.live[liveKey(projectRoot, runID)]
			engine.mu.Unlock()
			if !live {
				return run
			}
		}
		time.Sleep(5 * time.Millisecond)
	}

	run, err := engine.Get(context.Background(), projectRoot, runID)
	if err != nil {
		t.Fatalf("timed out and could not load run: %v", err)
	}
	t.Fatalf("timed out waiting for terminal cleanup; status %s, nodes %+v", run.Status, run.Nodes)
	return nil
}

func hasEvent(engine *Engine, projectRoot, runID, eventType string) (Event, bool) {
	events, err := engine.Replay(context.Background(), projectRoot, runID, 0)
	if err != nil {
		return Event{}, false
	}
	for _, event := range events {
		if event.Type == eventType {
			return event, true
		}
	}
	return Event{}, false
}
