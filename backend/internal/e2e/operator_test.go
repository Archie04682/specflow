package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/orchestrator"
)

// runEvent is the SSE payload the operator reacts to.
type runEvent struct {
	Seq    uint64         `json:"seq"`
	Type   string         `json:"type"`
	NodeID string         `json:"nodeId"`
	Data   map[string]any `json:"data"`
}

type operatorRules struct {
	// answer is the value returned for a question asked by the named node.
	answer map[string]string
	// approve decides a gate for the named node; a missing entry approves.
	approve map[string]bool
	// inject is sent once, shortly after the named node starts.
	inject map[string]string
	// injectOnTurn is sent once when the named node's first turn starts, so
	// the adapter steers rather than queues a follow-up.
	injectOnTurn map[string]string
}

// runOperator follows a run's SSE stream and acts as the human: it approves
// gates, answers questions, and injects once on start. It returns every event
// it saw when the stream ends. Errors are returned rather than raised so it can
// run in its own goroutine.
func runOperator(server *httptest.Server, project, runID string, rules operatorRules) ([]runEvent, error) {
	response, err := testClient.Get(runURL(server.URL, project, runID, "/events"))
	if err != nil {
		return nil, fmt.Errorf("open event stream: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open event stream: status %d", response.StatusCode)
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var events []runEvent
	injected := make(map[string]bool)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		var event runEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			return events, fmt.Errorf("decode event: %w", err)
		}
		events = append(events, event)

		switch event.Type {
		case "approval.required":
			decision := "approve"
			if allowed, ok := rules.approve[event.NodeID]; ok && !allowed {
				decision = "reject"
			}
			attempt, _ := event.Data["attempt"].(float64)
			status, body := doPost(runURL(server.URL, project, runID, "/approve"), map[string]any{
				"nodeId":   event.NodeID,
				"decision": decision,
				"attempt":  int(attempt),
			})
			if status != http.StatusOK {
				return events, fmt.Errorf("approve %s: status %d: %s", event.NodeID, status, body)
			}

		case "question.asked":
			id, _ := event.Data["id"].(string)
			value, ok := rules.answer[event.NodeID]
			if !ok {
				value = "42"
			}
			status, body := doPost(runURL(server.URL, project, runID, "/answers"), map[string]string{
				"questionId": id,
				"value":      value,
			})
			if status != http.StatusOK {
				return events, fmt.Errorf("answer %s: status %d: %s", id, status, body)
			}

		case "node.started":
			text, ok := rules.inject[event.NodeID]
			if !ok || injected[event.NodeID] {
				continue
			}
			injected[event.NodeID] = true
			status, body := doPost(runURL(server.URL, project, runID, "/injections"), map[string]string{
				"text": text,
			})
			if status != http.StatusOK {
				return events, fmt.Errorf("inject %s: status %d: %s", event.NodeID, status, body)
			}

		case "node.event":
			if kind, _ := event.Data["kind"].(string); kind != "turn_started" {
				continue
			}
			text, ok := rules.injectOnTurn[event.NodeID]
			key := "turn:" + event.NodeID
			if !ok || injected[key] {
				continue
			}
			injected[key] = true
			status, body := doPost(runURL(server.URL, project, runID, "/injections"), map[string]string{
				"text": text,
			})
			if status != http.StatusOK {
				return events, fmt.Errorf("inject %s on turn start: status %d: %s", event.NodeID, status, body)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("read event stream: %w", err)
	}
	return events, nil
}

// doPost is the operator's HTTP client. It is separate from postJSON because
// the operator runs outside the test goroutine and must return errors.
func doPost(target string, body any) (int, []byte) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, []byte(err.Error())
	}
	response, err := testClient.Post(target, "application/json", bytes.NewReader(encoded))
	if err != nil {
		return 0, []byte(err.Error())
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, []byte(err.Error())
	}
	return response.StatusCode, data
}

// interactiveAdapter asks one question, waits for one injection, and reports
// both so the test can assert what the operator delivered.
type interactiveAdapter struct {
	ask *harness.Ask

	mu        sync.Mutex
	answer    string
	injection string
}

func (a *interactiveAdapter) Descriptor() harness.Descriptor {
	return harness.Descriptor{ID: "scripted", Name: "Scripted", Version: "0.1.0", Protocol: harness.ProtocolVersion}
}

func (a *interactiveAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{Streaming: true, Questions: true, Injection: true, Abort: true}
}

func (a *interactiveAdapter) Probe(context.Context) (harness.ProbeResult, error) {
	return harness.ProbeResult{Capabilities: a.Capabilities()}, nil
}

func (a *interactiveAdapter) Run(ctx context.Context, _ harness.Request, host harness.Host) (harness.Result, error) {
	if a.ask != nil {
		answer, err := host.Ask(ctx, *a.ask)
		if err != nil {
			return canceled("ask: " + err.Error())
		}
		if answer.Cancelled {
			return canceled("question cancelled")
		}
		a.mu.Lock()
		a.answer = answer.Value
		a.mu.Unlock()
	}

	select {
	case injection := <-host.Injections():
		a.mu.Lock()
		a.injection = injection.Text
		a.mu.Unlock()
		if err := host.Emit(harness.Event{
			Kind:      harness.KindInjectionAck,
			Injection: &harness.InjectionAck{ID: injection.ID, Disposition: "delivered"},
		}); err != nil {
			return canceled("ack injection: " + err.Error())
		}
	case <-time.After(10 * time.Second):
		return canceled("no injection arrived")
	case <-ctx.Done():
		return canceled("execution cancelled")
	}

	return harness.Result{Status: harness.StatusCompleted, Text: "operator done"}, nil
}

func (a *interactiveAdapter) delivered() (answer, injection string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.answer, a.injection
}

func canceled(message string) (harness.Result, error) {
	result := harness.Result{
		Status:  harness.StatusCancelled,
		Failure: &harness.Failure{Class: harness.FailureCancelled, Message: message},
	}
	return result, result.Err()
}

func TestAutoOperatorDrivesQuestionApprovalAndInjection(t *testing.T) {
	adapter := &interactiveAdapter{
		ask: &harness.Ask{
			ID:      "q1",
			Kind:    harness.AskInput,
			Title:   "E2E question",
			Message: "What value should be recorded?",
		},
	}

	server, _, project := newServer(t, "scripted", adapter, map[string]string{
		"operator": `
id: operator
defaults: {harness: scripted}
nodes:
  - id: gate
    kind: agent
    prompt: ask and wait
    interactive: true
    allow_inject: true
    require_approval: true
`,
	})

	run := createRun(t, server, project, "operator", "exercise the operator")

	type result struct {
		events []runEvent
		err    error
	}
	done := make(chan result, 1)
	go func() {
		events, err := runOperator(server, project, run.ID, operatorRules{
			answer:  map[string]string{"gate": "42"},
			approve: map[string]bool{"gate": true},
			inject:  map[string]string{"gate": "use the second option"},
		})
		done <- result{events: events, err: err}
	}()

	var outcome result
	select {
	case outcome = <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("the operator never finished; run state: %+v", getRun(t, server, project, run.ID))
	}
	if outcome.err != nil {
		t.Fatalf("operator: %v", outcome.err)
	}

	finished := waitRun(t, server, project, run.ID, 10*time.Second)
	if finished.Status != orchestrator.RunCompleted {
		t.Fatalf("status = %s (%s)", finished.Status, finished.Error)
	}

	answer, injection := adapter.delivered()
	if answer != "42" {
		t.Fatalf("adapter received answer %q, want 42", answer)
	}
	if injection != "use the second option" {
		t.Fatalf("adapter received injection %q", injection)
	}

	assertEventOrder(t, outcome.events, []string{
		"approval.required",
		"approval.granted",
		"question.asked",
		"injection.received",
		"question.answered",
		"injection.acknowledged",
	})
}

// assertEventOrder requires the wanted types to appear as a subsequence.
func assertEventOrder(t *testing.T, events []runEvent, wanted []string) {
	t.Helper()

	next := 0
	for _, event := range events {
		if next < len(wanted) && event.Type == wanted[next] {
			next++
		}
	}
	if next != len(wanted) {
		types := make([]string, len(events))
		for i, event := range events {
			types[i] = event.Type
		}
		t.Fatalf("missing %q in event order; saw %v", wanted[next], types)
	}
}

// driveRun runs the operator in a goroutine and waits for the stream to close.
func driveRun(t *testing.T, server *httptest.Server, project, runID string, rules operatorRules) []runEvent {
	t.Helper()
	return driveRunFor(t, server, project, runID, rules, 120*time.Second)
}

// driveRunFor is driveRun with an explicit deadline for the full workflow.
func driveRunFor(t *testing.T, server *httptest.Server, project, runID string, rules operatorRules, timeout time.Duration) []runEvent {
	t.Helper()

	type outcome struct {
		events []runEvent
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		events, err := runOperator(server, project, runID, rules)
		done <- outcome{events: events, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("operator: %v", got.err)
		}
		return got.events
	case <-time.After(timeout):
		t.Fatalf("operator did not finish; run state: %+v", getRun(t, server, project, runID))
	}
	return nil
}

// collectEvents follows a run that needs no operator input.
func collectEvents(t *testing.T, server *httptest.Server, project, runID string) []runEvent {
	t.Helper()
	return driveRun(t, server, project, runID, operatorRules{})
}
