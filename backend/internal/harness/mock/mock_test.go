package mock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
)

type recordingHost struct {
	mu         sync.Mutex
	events     []harness.Event
	answer     harness.Answer
	askErr     error
	injections chan harness.Injection
	emitDelay  time.Duration
}

func (h *recordingHost) Emit(event harness.Event) error {
	if h.emitDelay > 0 {
		time.Sleep(h.emitDelay)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	return nil
}

func (h *recordingHost) Ask(context.Context, harness.Ask) (harness.Answer, error) {
	return h.answer, h.askErr
}

func (h *recordingHost) Injections() <-chan harness.Injection {
	return h.injections
}

func (h *recordingHost) recorded() []harness.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]harness.Event(nil), h.events...)
}

func TestRunEmitsEventsInOrder(t *testing.T) {
	host := &recordingHost{}
	adapter := New(Script{Events: []harness.Event{
		{Kind: harness.KindTurnStarted},
		{Kind: harness.KindTextDelta, Text: "hello"},
		{Kind: harness.KindTextDone, Text: "hello"},
	}})

	result, err := adapter.Run(context.Background(), harness.Request{}, host)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	events := host.recorded()
	if len(events) != 3 {
		t.Fatalf("received %d events, want 3", len(events))
	}
	want := []harness.Kind{harness.KindTurnStarted, harness.KindTextDelta, harness.KindTextDone}
	for i, kind := range want {
		if events[i].Kind != kind {
			t.Fatalf("event %d = %q, want %q", i, events[i].Kind, kind)
		}
		if events[i].Seq != uint64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, events[i].Seq, i+1)
		}
	}
}

// TestBlockingEmitKeepsOrderAndLosesNothing pins the backpressure contract: a
// slow host slows the adapter but does not reorder or drop events.
func TestBlockingEmitKeepsOrderAndLosesNothing(t *testing.T) {
	delay := 10 * time.Millisecond
	host := &recordingHost{emitDelay: delay}
	adapter := New(Script{Events: []harness.Event{
		{Kind: harness.KindTextDelta, Text: "a"},
		{Kind: harness.KindTextDelta, Text: "b"},
		{Kind: harness.KindTextDelta, Text: "c"},
		{Kind: harness.KindTextDelta, Text: "d"},
	}})

	started := time.Now()
	result, err := adapter.Run(context.Background(), harness.Request{}, host)
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	events := host.recorded()
	if len(events) != 4 {
		t.Fatalf("received %d events, want 4", len(events))
	}
	for i, event := range events {
		if event.Seq != uint64(i+1) {
			t.Fatalf("event %d seq = %d, want %d (events were reordered or duplicated)", i, event.Seq, i+1)
		}
	}
	if elapsed < 4*delay {
		t.Fatalf("Run took %v, want at least %v (Emit was not allowed to block)", elapsed, 4*delay)
	}
}

func TestAskAnswerReachesTheResult(t *testing.T) {
	host := &recordingHost{answer: harness.Answer{ID: "q1", Value: "yes, use JWT"}}
	adapter := New(Script{
		Ask:    &harness.Ask{ID: "q1", Kind: harness.AskConfirm, Message: "Use JWT?"},
		Result: harness.Result{Status: harness.StatusCompleted},
	})

	result, err := adapter.Run(context.Background(), harness.Request{}, host)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Text != "yes, use JWT" {
		t.Fatalf("result text = %q, want the answer", result.Text)
	}
}

func TestCancelledAskDoesNotDeadlock(t *testing.T) {
	host := &recordingHost{answer: harness.Answer{ID: "q1", Cancelled: true}}
	adapter := New(Script{
		Ask: &harness.Ask{ID: "q1", Kind: harness.AskInput},
	})

	done := make(chan struct{})
	var result harness.Result
	var err error
	go func() {
		defer close(done)
		result, err = adapter.Run(context.Background(), harness.Request{}, host)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after a cancelled ask")
	}

	if result.Status != harness.StatusCancelled {
		t.Fatalf("status = %q, want cancelled", result.Status)
	}
	if err == nil {
		t.Fatal("Run returned nil error for a cancelled execution")
	}
	var failure *harness.Failure
	if !errors.As(err, &failure) || failure.Class != harness.FailureCancelled {
		t.Fatalf("error = %v, want a cancelled failure", err)
	}
}

func TestInjectionIsAcknowledged(t *testing.T) {
	injections := make(chan harness.Injection, 1)
	injections <- harness.Injection{ID: "i1", Text: "use PASETO instead"}
	host := &recordingHost{injections: injections}

	adapter := New(Script{WaitForInjection: true})
	result, err := adapter.Run(context.Background(), harness.Request{}, host)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	events := host.recorded()
	if len(events) != 1 {
		t.Fatalf("received %d events, want 1 acknowledgement", len(events))
	}
	ack := events[0]
	if ack.Kind != harness.KindInjectionAck {
		t.Fatalf("event kind = %q, want %q", ack.Kind, harness.KindInjectionAck)
	}
	if ack.Injection == nil || ack.Injection.ID != "i1" {
		t.Fatalf("ack = %#v, want the injected id", ack.Injection)
	}
}

func TestInjectionWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	host := &recordingHost{injections: make(chan harness.Injection)}
	adapter := New(Script{WaitForInjection: true})

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	result, err := adapter.Run(ctx, harness.Request{}, host)
	if err == nil {
		t.Fatal("Run returned nil error after cancellation")
	}
	if result.Status != harness.StatusCancelled {
		t.Fatalf("status = %q, want cancelled", result.Status)
	}
}

func TestScriptWritesFilesOnlyAtTheNamedNode(t *testing.T) {
	workdir := t.TempDir()
	adapter := New(Script{
		Writes:   map[string]string{"nested/file.txt": "hello"},
		WritesOn: "propose",
	})
	host := &recordingHost{}

	if _, err := adapter.Run(context.Background(), harness.Request{NodeID: "explore", Workdir: workdir}, host); err != nil {
		t.Fatalf("Run on another node: %v", err)
	}
	path := filepath.Join(workdir, "nested", "file.txt")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a node the script does not name wrote files")
	}

	if _, err := adapter.Run(context.Background(), harness.Request{NodeID: "propose", Workdir: workdir}, host); err != nil {
		t.Fatalf("Run on the named node: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("content = %q", data)
	}
}
