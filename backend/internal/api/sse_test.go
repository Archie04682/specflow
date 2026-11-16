package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/mock"
	"example.com/specflow/backend/internal/orchestrator"
)

const sseWorkflow = `
id: demo
defaults: {harness: mock}
nodes:
  - {id: a, kind: agent, prompt: work}
`

func newSSEServer(t *testing.T, script mock.Script) (*httptest.Server, string) {
	t.Helper()
	return newSSEServerWithWorkflow(t, script, sseWorkflow)
}

func newSSEServerWithWorkflow(t *testing.T, script mock.Script, workflow string) (*httptest.Server, string) {
	t.Helper()

	project := newRepo(t)
	workflowPath := filepath.Join(project, ".specflow", "workflows", "test.yaml")
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workflowPath, []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter { return mock.New(script) }); err != nil {
		t.Fatal(err)
	}
	engine := orchestrator.New(registry)
	t.Cleanup(engine.Close)

	server := httptest.NewServer(NewRouterWithDeps(fstestMapFS(), registry, engine))
	t.Cleanup(server.Close)
	return server, project
}

func textEvents(count int) []harness.Event {
	events := make([]harness.Event, 0, count)
	for i := 0; i < count; i++ {
		events = append(events, harness.Event{Kind: harness.KindTextDelta, Text: "chunk"})
	}
	return events
}

func sseResponse(t *testing.T, client *http.Client, target string, lastEventID uint64) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lastEventID > 0 {
		req.Header.Set("Last-Event-ID", formatUint(lastEventID))
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func formatUint(value uint64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}

func readSSE(t *testing.T, body io.Reader) []orchestrator.Event {
	t.Helper()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var events []orchestrator.Event
	var data string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && data != "":
			var event orchestrator.Event
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				t.Errorf("decode SSE event: %v", err)
				data = ""
				continue
			}
			events = append(events, event)
			data = ""
		}
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("read SSE: %v", err)
	}
	return events
}

func eventTypes(events []orchestrator.Event) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func waitRunStatus(t *testing.T, server *httptest.Server, project, runID string, want orchestrator.RunStatus) orchestrator.Run {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, body := request(t, http.MethodGet, runURL(server, project, runID, ""), nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("get run: %d %s", response.StatusCode, body)
		}
		var run orchestrator.Run
		if err := json.Unmarshal(body, &run); err != nil {
			t.Fatal(err)
		}
		if run.Status == want {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run did not reach %s", want)
	return orchestrator.Run{}
}

func answerQuestion(t *testing.T, server *httptest.Server, project, runID, questionID string) {
	t.Helper()

	answer, body := request(t, http.MethodPost, runURL(server, project, runID, "/answers"), map[string]any{
		"questionId": questionID,
		"value":      "yes",
	})
	if answer.StatusCode != http.StatusOK {
		t.Fatalf("answer: %d %s", answer.StatusCode, body)
	}
}

func TestEventsStreamLiveEndsOnTerminal(t *testing.T) {
	server, project := newSSEServer(t, mock.Script{
		Events: textEvents(2),
		Ask:    &harness.Ask{ID: "q", Title: "Question"},
	})
	run := createRun(t, server, project)
	waitRunStatus(t, server, project, run.ID, orchestrator.RunWaitingAnswer)

	client := &http.Client{Timeout: 5 * time.Second}
	response := sseResponse(t, client, runURL(server, project, run.ID, "/events"), 0)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d", response.StatusCode)
	}

	answerQuestion(t, server, project, run.ID, "q")
	waitCompleted(t, server, project, run.ID)

	events := readSSE(t, response.Body)
	types := eventTypes(events)
	for _, required := range []string{"run.started", "node.started", "node.event", "node.completed", "run.completed"} {
		found := false
		for _, eventType := range types {
			if eventType == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s in %v", required, types)
		}
	}
	if types[len(types)-1] != "run.completed" {
		t.Fatalf("last event = %s", types[len(types)-1])
	}
}

func TestEventsStreamTwoSubscribers(t *testing.T) {
	server, project := newSSEServer(t, mock.Script{
		Events: textEvents(2),
		Ask:    &harness.Ask{ID: "q", Title: "Question"},
	})
	run := createRun(t, server, project)
	waitRunStatus(t, server, project, run.ID, orchestrator.RunWaitingAnswer)

	client := &http.Client{Timeout: 5 * time.Second}
	first := sseResponse(t, client, runURL(server, project, run.ID, "/events"), 0)
	defer first.Body.Close()
	second := sseResponse(t, client, runURL(server, project, run.ID, "/events"), 0)
	defer second.Body.Close()
	answerQuestion(t, server, project, run.ID, "q")
	waitCompleted(t, server, project, run.ID)

	var wg sync.WaitGroup
	results := make([][]orchestrator.Event, 2)
	for index, response := range []*http.Response{first, second} {
		wg.Add(1)
		go func(index int, response *http.Response) {
			defer wg.Done()
			results[index] = readSSE(t, response.Body)
		}(index, response)
	}
	wg.Wait()

	if len(results[0]) == 0 || len(results[1]) == 0 {
		t.Fatalf("streams = %d, %d events", len(results[0]), len(results[1]))
	}
	if got, want := strings.Join(eventTypes(results[0]), ","), strings.Join(eventTypes(results[1]), ","); got != want {
		t.Fatalf("streams differ:\n%s\n%s", got, want)
	}
}

func TestEventsStreamHistoryThenLive(t *testing.T) {
	server, project := newSSEServer(t, mock.Script{
		Events: textEvents(2),
		Ask:    &harness.Ask{ID: "q", Title: "Question"},
	})
	run := createRun(t, server, project)
	waitRunStatus(t, server, project, run.ID, orchestrator.RunWaitingAnswer)

	client := &http.Client{Timeout: 5 * time.Second}
	response := sseResponse(t, client, runURL(server, project, run.ID, "/events"), 1)
	defer response.Body.Close()

	answerQuestion(t, server, project, run.ID, "q")
	waitCompleted(t, server, project, run.ID)

	events := readSSE(t, response.Body)
	if len(events) == 0 {
		t.Fatal("history + live stream was empty")
	}
	if events[0].Seq <= 1 {
		t.Fatalf("first event seq = %d; the cursor was not honoured", events[0].Seq)
	}
	if events[len(events)-1].Type != "run.completed" {
		t.Fatalf("last event = %s", events[len(events)-1].Type)
	}
}

func TestEventsStreamTerminalTailReturnsNoContent(t *testing.T) {
	server, project := newSSEServer(t, mock.Script{})
	run := createRun(t, server, project)
	waitCompleted(t, server, project, run.ID)

	client := &http.Client{Timeout: 5 * time.Second}
	response := sseResponse(t, client, runURL(server, project, run.ID, "/events"), 0)
	events := readSSE(t, response.Body)
	response.Body.Close()
	if len(events) == 0 {
		t.Fatal("completed run had no history")
	}
	last := events[len(events)-1].Seq

	tail := sseResponse(t, client, runURL(server, project, run.ID, "/events"), last)
	defer tail.Body.Close()
	if tail.StatusCode != http.StatusNoContent {
		t.Fatalf("tail status = %d", tail.StatusCode)
	}
}

func TestEventsStreamHeartbeat(t *testing.T) {
	previous := sseHeartbeatInterval
	sseHeartbeatInterval = 10 * time.Millisecond
	defer func() { sseHeartbeatInterval = previous }()

	server, project := newSSEServer(t, mock.Script{
		Events:    textEvents(10),
		EmitDelay: 100 * time.Millisecond,
	})
	run := createRun(t, server, project)

	client := &http.Client{Timeout: 5 * time.Second}
	response := sseResponse(t, client, runURL(server, project, run.ID, "/events"), 0)
	defer response.Body.Close()

	lines := make(chan string, 32)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatal("stream ended before a heartbeat")
			}
			if line == ": keep-alive" {
				return
			}
		case <-deadline:
			t.Fatal("no heartbeat was sent")
		}
	}
}

func TestEventsStreamDisconnectCancelsHandler(t *testing.T) {
	server, project := newSSEServer(t, mock.Script{
		Events:    textEvents(20),
		EmitDelay: 50 * time.Millisecond,
	})
	run := createRun(t, server, project)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runURL(server, project, run.ID, "/events"), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	response.Body.Close()

	// Give the handler time to observe the cancellation; the run must still finish.
	waitCompleted(t, server, project, run.ID)
}

func TestEventsStreamRepeatedDisconnects(t *testing.T) {
	server, project := newSSEServer(t, mock.Script{
		Events:    textEvents(20),
		EmitDelay: 20 * time.Millisecond,
	})
	run := createRun(t, server, project)

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	before := runtime.NumGoroutine()
	for i := 0; i < 8; i++ {
		response := sseResponse(t, client, runURL(server, project, run.ID, "/events"), 0)
		response.Body.Close()
	}
	waitCompleted(t, server, project, run.ID)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= before+10 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines after disconnects = %d, baseline = %d", runtime.NumGoroutine(), before)
}

func TestEventsStreamUnknownRunIsNotFound(t *testing.T) {
	server, project := newSSEServer(t, mock.Script{})
	response, body := request(t, http.MethodGet, runURL(server, project, "missing", "/events"), nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d: %s", response.StatusCode, body)
	}
}

func TestInteractiveEventsStreamInOrder(t *testing.T) {
	server, project := newSSEServerWithWorkflow(t, mock.Script{
		Ask:              &harness.Ask{ID: "q1", Kind: harness.AskInput},
		WaitForInjection: true,
	}, `
id: interactive
defaults: {harness: mock}
nodes:
  - {id: a, kind: agent, prompt: work, require_approval: true, allow_inject: true, approval_timeout: 5s}
`)
	run := createRun(t, server, project)
	waitRunStatus(t, server, project, run.ID, orchestrator.RunWaitingApproval)

	client := &http.Client{Timeout: 5 * time.Second}
	response := sseResponse(t, client, runURL(server, project, run.ID, "/events"), 0)
	defer response.Body.Close()

	approved, body := request(t, http.MethodPost, runURL(server, project, run.ID, "/approve"), map[string]any{
		"nodeId": "a", "decision": "approve", "attempt": 1,
	})
	if approved.StatusCode != http.StatusOK {
		t.Fatalf("approve: %d %s", approved.StatusCode, body)
	}

	waitRunStatus(t, server, project, run.ID, orchestrator.RunWaitingAnswer)
	answerQuestion(t, server, project, run.ID, "q1")

	injected, body := request(t, http.MethodPost, runURL(server, project, run.ID, "/injections"), map[string]string{
		"text": "use encoding/csv",
	})
	if injected.StatusCode != http.StatusOK {
		t.Fatalf("inject: %d %s", injected.StatusCode, body)
	}
	var injectResponse struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &injectResponse); err != nil {
		t.Fatalf("decode inject response: %v", err)
	}
	if injectResponse.Status != "queued" {
		t.Fatalf("inject status = %q", injectResponse.Status)
	}

	waitCompleted(t, server, project, run.ID)
	events := readSSE(t, response.Body)
	types := eventTypes(events)

	order := []string{
		"approval.required",
		"approval.granted",
		"question.asked",
		"question.answered",
		"injection.received",
		"injection.acknowledged",
		"run.completed",
	}
	position := make(map[string]int, len(types))
	for index, eventType := range types {
		if _, seen := position[eventType]; !seen {
			position[eventType] = index
		}
	}
	last := -1
	for _, eventType := range order {
		index, ok := position[eventType]
		if !ok {
			t.Fatalf("missing %s in %v", eventType, types)
		}
		if index <= last {
			t.Fatalf("%s is out of order: %v", eventType, types)
		}
		last = index
	}
}
