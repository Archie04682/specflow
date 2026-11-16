package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/pi"
	"example.com/specflow/backend/internal/orchestrator"
)

// TestRung1OneNode proves the whole path against the real binary: argv, the
// RPC handshake, streamed events, the terminal result, worktree cleanup, and
// process shutdown.
func TestRung1OneNode(t *testing.T) {
	model, binary := requireRealPi(t)
	shim, pidfile := writePiShim(t, binary, shimPass)
	adapter := newPiAdapter(t, shim)

	workflow := fmt.Sprintf(`
id: rung1
defaults:
  harness: pi
  model: {id: %s, thinking: off}
nodes:
  - id: smoke
    kind: agent
    prompt: Reply with exactly the word ok. Do not use any tools.
`, model)

	server, _, project := newServer(t, "pi", adapter, map[string]string{"rung1": workflow})
	baseline := runtime.NumGoroutine()
	descriptors, haveFDs := openFDCount()

	run := createRun(t, server, project, "rung1", "reply ok")
	events := collectEvents(t, server, project, run.ID)
	done := waitRun(t, server, project, run.ID, 20*time.Second)

	if done.Status != orchestrator.RunCompleted {
		t.Fatalf("run status = %s (%s); node text: %q", done.Status, done.Error, done.Node("write").Text)
	}
	node := done.Node("smoke")
	if node == nil || node.Status != orchestrator.NodeCompleted {
		t.Fatalf("node = %+v, want completed", node)
	}
	if !strings.Contains(strings.ToLower(node.Text), "ok") {
		t.Fatalf("node text = %q, want it to contain ok", node.Text)
	}
	if node.Usage.TotalTokens == 0 {
		t.Log("provider reported no usage; the adapter tolerates that")
	}

	assertEventKinds(t, events)
	assertStartupArgs(t, readShimArgs(t, pidfile), model)

	if _, err := os.Stat(filepath.Join(project, ".specflow", "worktrees", run.ID)); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after completion: %v", err)
	}

	assertProcessGone(t, readPID(t, pidfile))
	if haveFDs {
		assertNoFDLeak(t, descriptors)
	}
	assertNoGoroutineLeak(t, baseline)
}

// assertEventKinds checks the stream carried the lifecycle and the telemetry a
// single completed node should produce.
func assertEventKinds(t *testing.T, events []runEvent) {
	t.Helper()

	seen := make(map[string]bool)
	telemetry := make(map[string]bool)
	for _, event := range events {
		seen[event.Type] = true
		if event.Type == "node.event" {
			if kind, ok := event.Data["kind"].(string); ok {
				telemetry[kind] = true
			}
		}
	}

	for _, want := range []string{"run.started", "node.started", "node.completed", "run.completed"} {
		if !seen[want] {
			t.Fatalf("missing %q in the event stream", want)
		}
	}
	for _, want := range []string{"turn_started", "text_delta", "turn_finished"} {
		if !telemetry[want] {
			t.Fatalf("missing telemetry %q; saw %v", want, telemetry)
		}
	}
	if seen["node.failed"] {
		t.Fatal("a completed run published node.failed")
	}
}

// assertStartupArgs proves the adapter built the documented Pi command line.
func assertStartupArgs(t *testing.T, args []string, model string) {
	t.Helper()

	for _, pair := range [][2]string{
		{"--mode", "rpc"},
		{"--model", model},
		{"--thinking", "off"},
		{"--tools", "+specflow_ask"},
	} {
		value, ok := argValue(args, pair[0])
		if !ok {
			t.Fatalf("args %v do not contain %s", args, pair[0])
		}
		if value != pair[1] {
			t.Fatalf("%s = %q, want %q", pair[0], value, pair[1])
		}
	}
	for _, flag := range []string{"--no-session", "--no-approve", "-e"} {
		if !containsArg(args, flag) {
			t.Fatalf("args %v do not contain %s", args, flag)
		}
	}
}

func argValue(args []string, flag string) (string, bool) {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func containsArg(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

// TestRung2SkillWritesFile proves deterministic skill binding, tool and file
// telemetry, a checkpoint commit, and the file_exists postcondition.
func TestRung2SkillWritesFile(t *testing.T) {
	model, binary := requireRealPi(t)
	shim, pidfile := writePiShim(t, binary, shimPass)
	adapter := newPiAdapter(t, shim)

	workflow := fmt.Sprintf(`
id: rung2
defaults:
  harness: pi
  model: {id: %s, thinking: off}
nodes:
  - id: write
    kind: agent
    skill: write-file
    prompt: |
      Call the write tool now with path exactly "proof.txt" (lowercase, .txt)
      and content exactly "written by pi". Do not create PROOF.md or any other
      filename. Then reply done.
    tools: [write]
    postcondition:
      type: file_exists
      path: proof.txt
`, model)

	server, _, project := newServer(t, "pi", adapter, map[string]string{"rung2": workflow})
	run := createRun(t, server, project, "rung2", "write the proof file")
	events := collectEvents(t, server, project, run.ID)
	done := waitRun(t, server, project, run.ID, 40*time.Second)

	if done.Status != orchestrator.RunCompleted {
		t.Fatalf("run status = %s (%s); node text: %q", done.Status, done.Error, done.Node("write").Text)
	}
	node := done.Node("write")
	if node == nil || node.Status != orchestrator.NodeCompleted {
		t.Fatalf("node = %+v, want completed", node)
	}
	if node.Checkpoint == "" {
		t.Fatal("node has no checkpoint commit")
	}

	var toolStarted, toolFinished, fileChanged, postconditionMet bool
	for _, event := range events {
		if event.Type == "node.completed" && event.NodeID == "write" {
			postconditionMet = event.Data["postcondition"] == "met"
		}
		if event.Type != "node.event" {
			continue
		}
		kind, _ := event.Data["kind"].(string)
		switch kind {
		case "tool_started":
			toolStarted = true
			t.Logf("tool_started: %+v", event.Data["tool"])
		case "tool_finished":
			toolFinished = true
			t.Logf("tool_finished: %+v", event.Data["tool"])
		case "file_changed":
			fileChanged = true
			t.Logf("file_changed: %+v", event.Data["file"])
		}
	}
	if !postconditionMet {
		t.Fatal("node.completed did not report the postcondition as met")
	}
	for name, seen := range map[string]bool{
		"tool_started":  toolStarted,
		"tool_finished": toolFinished,
		"file_changed":  fileChanged,
	} {
		if !seen {
			t.Fatalf("missing %q telemetry for the write", name)
		}
	}

	// The worktree is gone, but the checkpoint commit is on the run branch.
	output, err := exec.Command("git", "-C", project, "show", "specflow/run-"+run.ID+":proof.txt").Output()
	if err != nil {
		t.Fatalf("read proof.txt from the run branch: %v", err)
	}
	if strings.TrimSpace(string(output)) == "" {
		t.Fatal("proof.txt is empty")
	}

	assertProcessGone(t, readPID(t, pidfile))
}

// TestRung3ChangeChain proves the worktree persists across nodes, the change
// name is resolved from the first node's postcondition, checkpoints are
// ordered, and both change-relative postconditions run against real state.
func TestRung3ChangeChain(t *testing.T) {
	model, binary := requireRealPi(t)
	shim, pidfile := writePiShim(t, binary, shimPass)
	adapter := newPiAdapter(t, shim)

	workflow := fmt.Sprintf(`
id: rung3
defaults:
  harness: pi
  model: {id: %s, thinking: off}
nodes:
  - id: propose
    kind: agent
    skill: create-change
    prompt: |
      Call the write tool with path exactly "openspec/changes/demo-change/tasks.md"
      and content exactly:

      # Tasks

      - [ ] 1.1 Do the thing

      Then reply done. Do not rename the change or the file.
    tools: [write]
    postcondition:
      type: new_change
      description: the proposal creates a change

  - id: apply
    kind: agent
    skill: complete-tasks
    prompt: |
      The change is "{{change}}". Call the write tool with path exactly
      "openspec/changes/{{change}}/tasks.md" and content exactly:

      # Tasks

      - [x] 1.1 Do the thing

      Then reply done.
    tools: [write]
    postcondition:
      type: tasks_complete
      change: "{{change}}"
      description: every task is checked

edges:
  - {from: propose, to: apply}
`, model)

	server, _, project := newServer(t, "pi", adapter, map[string]string{"rung3": workflow})
	run := createRun(t, server, project, "rung3", "create and finish a change")
	events := collectEvents(t, server, project, run.ID)
	done := waitRun(t, server, project, run.ID, 60*time.Second)

	if done.Status != orchestrator.RunCompleted {
		t.Fatalf("run status = %s (%s); apply text: %q", done.Status, done.Error, done.Node("apply").Text)
	}
	if done.Change != "demo-change" {
		t.Fatalf("resolved change = %q, want demo-change", done.Change)
	}
	if done.Node("propose").Status != orchestrator.NodeCompleted || done.Node("apply").Status != orchestrator.NodeCompleted {
		t.Fatalf("nodes = %+v, %+v, want both completed", done.Node("propose"), done.Node("apply"))
	}

	proposeCheckpoint := done.Checkpoints["propose"]
	applyCheckpoint := done.Checkpoints["apply"]
	if proposeCheckpoint == "" || applyCheckpoint == "" {
		t.Fatalf("checkpoints = %+v, want two commits", done.Checkpoints)
	}
	if proposeCheckpoint == applyCheckpoint {
		t.Fatal("the second node did not advance the branch")
	}

	if body := gitShow(t, project, proposeCheckpoint, "openspec/changes/demo-change/tasks.md"); !strings.Contains(body, "- [ ]") {
		t.Fatalf("propose checkpoint tasks = %q, want the unfinished task", body)
	}
	if body := gitShow(t, project, applyCheckpoint, "openspec/changes/demo-change/tasks.md"); !strings.Contains(body, "- [x]") {
		t.Fatalf("apply checkpoint tasks = %q, want the completed task", body)
	}

	met := make(map[string]bool)
	for _, event := range events {
		if event.Type == "node.completed" && event.Data["postcondition"] == "met" {
			met[event.NodeID] = true
		}
	}
	if !met["propose"] || !met["apply"] {
		t.Fatalf("postconditions met = %v, want both nodes", met)
	}

	assertProcessGone(t, readPID(t, pidfile))
}

func gitShow(t *testing.T, project, ref, path string) string {
	t.Helper()

	output, err := exec.Command("git", "-C", project, "show", ref+":"+path).Output()
	if err != nil {
		t.Fatalf("git show %s:%s: %v", ref, path, err)
	}
	return string(output)
}

// TestRung4aAsk proves the embedded bridge round trip against real Pi: the
// model asks, the operator answers through the API, and the execution
// continues with that answer.
func TestRung4aAsk(t *testing.T) {
	model, binary := requireRealPi(t)
	shim, pidfile := writePiShim(t, binary, shimPass)
	adapter := newPiAdapter(t, shim)

	workflow := fmt.Sprintf(`
id: rung4a
defaults:
  harness: pi
  model: {id: %s, thinking: off}
nodes:
  - id: ask
    kind: agent
    skill: ask-and-record
    prompt: |
      You MUST call the specflow_ask tool exactly once before writing anything.
      Ask exactly "What value should be recorded?" with kind "input" and title
      "E2E question". Wait for the answer. Then call the write tool to create
      answer.txt containing exactly the answer text, and reply done.
    tools: [write]
    interactive: true
    postcondition:
      type: file_exists
      path: answer.txt
`, model)

	server, _, project := newServer(t, "pi", adapter, map[string]string{"rung4a": workflow})
	run := createRun(t, server, project, "rung4a", "ask the operator")
	events := driveRun(t, server, project, run.ID, operatorRules{
		answer: map[string]string{"ask": "42"},
	})
	done := waitRun(t, server, project, run.ID, 60*time.Second)

	if done.Status != orchestrator.RunCompleted {
		t.Fatalf("run status = %s (%s); node text: %q", done.Status, done.Error, done.Node("ask").Text)
	}

	asked, askedOK := findEvent(events, "question.asked")
	answered, answeredOK := findEvent(events, "question.answered")
	if !askedOK || !answeredOK {
		t.Fatalf("question events missing: asked=%v answered=%v", askedOK, answeredOK)
	}
	if asked.Data["id"] != answered.Data["id"] {
		t.Fatalf("question ids differ: %v vs %v", asked.Data["id"], answered.Data["id"])
	}
	if answered.Data["value"] != "42" {
		t.Fatalf("answered value = %v, want 42", answered.Data["value"])
	}

	body := gitShow(t, project, "specflow/run-"+run.ID, "answer.txt")
	if !strings.Contains(body, "42") {
		t.Fatalf("answer.txt = %q, want the delivered answer", body)
	}

	assertProcessGone(t, readPID(t, pidfile))
}

func findEvent(events []runEvent, eventType string) (runEvent, bool) {
	for _, event := range events {
		if event.Type == eventType {
			return event, true
		}
	}
	return runEvent{}, false
}

// TestRung4bInject proves a clarification is steered into a running turn and
// acknowledged, and that the execution finishes normally afterwards.
func TestRung4bInject(t *testing.T) {
	model, binary := requireRealPi(t)
	shim, pidfile := writePiShim(t, binary, shimPass)
	adapter := newPiAdapter(t, shim)

	workflow := fmt.Sprintf(`
id: rung4b
defaults:
  harness: pi
  model: {id: %s, thinking: off}
nodes:
  - id: slow
    kind: agent
    skill: slow-bash
    prompt: |
      Call the bash tool with the command "sleep 8" now. While it runs, a
      clarification may arrive. When it does, follow it exactly.
    tools: [bash]
    allow_inject: true
`, model)

	server, _, project := newServer(t, "pi", adapter, map[string]string{"rung4b": workflow})
	run := createRun(t, server, project, "rung4b", "inject while a tool runs")
	events := driveRun(t, server, project, run.ID, operatorRules{
		injectOnTurn: map[string]string{
			"slow": "When the command finishes, reply with exactly the word INJECTED.",
		},
	})
	done := waitRun(t, server, project, run.ID, 90*time.Second)

	if done.Status != orchestrator.RunCompleted {
		t.Fatalf("run status = %s (%s); node text: %q", done.Status, done.Error, done.Node("slow").Text)
	}

	received, receivedOK := findEvent(events, "injection.received")
	if !receivedOK || received.Data["text"] == "" {
		t.Fatalf("injection.received missing or empty: %+v", received)
	}
	ack, ackOK := findEvent(events, "injection.acknowledged")
	if !ackOK {
		t.Fatal("injection.acknowledged missing")
	}
	if disposition, _ := ack.Data["disposition"].(string); disposition == "" {
		t.Fatalf("injection acknowledgement has no disposition: %+v", ack.Data)
	}

	if !strings.Contains(strings.ToUpper(done.Node("slow").Text), "INJECTED") {
		t.Fatalf("node text = %q, want the injected instruction to be visible", done.Node("slow").Text)
	}

	assertProcessGone(t, readPID(t, pidfile))
}

// TestRung4cApproval proves the gate pauses before the node starts: approval
// runs the node, rejection cancels the run without spawning Pi.
func TestRung4cApproval(t *testing.T) {
	model, binary := requireRealPi(t)

	workflow := fmt.Sprintf(`
id: rung4c
defaults:
  harness: pi
  model: {id: %s, thinking: off}
nodes:
  - id: gate
    kind: agent
    skill: write-file
    prompt: |
      Call the write tool now with path exactly "proof.txt" (lowercase, .txt)
      and content exactly "written by pi". Then reply done.
    tools: [write]
    require_approval: true
    postcondition:
      type: file_exists
      path: proof.txt
`, model)

	t.Run("approve", func(t *testing.T) {
		shim, pidfile := writePiShim(t, binary, shimPass)
		adapter := newPiAdapter(t, shim)
		server, _, project := newServer(t, "pi", adapter, map[string]string{"rung4c": workflow})

		run := createRun(t, server, project, "rung4c", "approve the gate")
		events := driveRun(t, server, project, run.ID, operatorRules{
			approve: map[string]bool{"gate": true},
		})
		done := waitRun(t, server, project, run.ID, 60*time.Second)

		if done.Status != orchestrator.RunCompleted {
			t.Fatalf("run status = %s (%s)", done.Status, done.Error)
		}
		assertEventOrder(t, events, []string{"approval.required", "approval.granted", "node.started", "node.completed"})
		assertProcessGone(t, readPID(t, pidfile))
	})

	t.Run("reject", func(t *testing.T) {
		shim, pidfile := writePiShim(t, binary, shimPass)
		adapter := newPiAdapter(t, shim)
		server, _, project := newServer(t, "pi", adapter, map[string]string{"rung4c": workflow})

		run := createRun(t, server, project, "rung4c", "reject the gate")
		events := driveRun(t, server, project, run.ID, operatorRules{
			approve: map[string]bool{"gate": false},
		})
		done := waitRun(t, server, project, run.ID, 30*time.Second)

		if done.Status != orchestrator.RunCancelled {
			t.Fatalf("run status = %s, want cancelled (%s)", done.Status, done.Error)
		}
		if node := done.Node("gate"); node.Status == orchestrator.NodeCompleted {
			t.Fatalf("a rejected node completed: %+v", node)
		}
		assertEventOrder(t, events, []string{"approval.required", "approval.rejected"})
		for _, event := range events {
			if event.Type == "node.started" {
				t.Fatal("a rejected gate started the node")
			}
		}
		if _, err := os.Stat(pidfile); !os.IsNotExist(err) {
			t.Fatalf("a rejected gate spawned Pi: %v", err)
		}
	})
}

// TestRung5StartupFailures proves the two permanent startup problems are
// classified before any model work: an unknown model is configuration, absent
// credentials are authentication, and neither is retryable.
func TestRung5StartupFailures(t *testing.T) {
	binary := requireRealPiBinary(t)

	t.Run("unknown model", func(t *testing.T) {
		adapter := newPiAdapter(t, binary)
		workflow := `
id: badmodel
defaults: {harness: pi}
nodes:
  - id: bad
    kind: agent
    model: {id: definitely/not-a-model}
    prompt: say ok
`
		server, _, project := newServer(t, "pi", adapter, map[string]string{"badmodel": workflow})
		run := createRun(t, server, project, "badmodel", "use an unknown model")
		events := collectEvents(t, server, project, run.ID)
		done := waitRun(t, server, project, run.ID, 40*time.Second)

		if done.Status != orchestrator.RunFailed {
			t.Fatalf("run status = %s (%s)", done.Status, done.Error)
		}
		node := done.Node("bad")
		if node.FailureClass != string(harness.FailureConfig) {
			t.Fatalf("failure class = %q, want config", node.FailureClass)
		}
		failed, ok := failureEvent(events, "bad")
		if !ok {
			t.Fatal("missing node.failed event")
		}
		if retryable, _ := failed.Data["retryable"].(bool); retryable {
			t.Fatalf("an unknown model was reported retryable: %+v", failed.Data)
		}
	})

	t.Run("missing credentials", func(t *testing.T) {
		t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
		clearProviderKeys(t)

		adapter := newPiAdapter(t, binary)
		workflow := `
id: nokey
defaults: {harness: pi}
nodes:
  - id: bad
    kind: agent
    prompt: say ok
`
		server, _, project := newServer(t, "pi", adapter, map[string]string{"nokey": workflow})
		run := createRun(t, server, project, "nokey", "run without credentials")
		events := collectEvents(t, server, project, run.ID)
		done := waitRun(t, server, project, run.ID, 40*time.Second)

		if done.Status != orchestrator.RunFailed {
			t.Fatalf("run status = %s (%s)", done.Status, done.Error)
		}
		node := done.Node("bad")
		if node.FailureClass != string(harness.FailureAuth) {
			t.Fatalf("failure class = %q, want auth (%s)", node.FailureClass, node.Error)
		}
		failed, ok := failureEvent(events, "bad")
		if !ok {
			t.Fatal("missing node.failed event")
		}
		if retryable, _ := failed.Data["retryable"].(bool); retryable {
			t.Fatalf("missing credentials were reported retryable: %+v", failed.Data)
		}
	})
}

// clearProviderKeys removes ambient provider credentials for a test that must
// observe the unauthenticated path.
func clearProviderKeys(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY",
		"DEEPSEEK_API_KEY", "OPENROUTER_API_KEY",
	} {
		t.Setenv(key, "")
	}
}

func failureEvent(events []runEvent, nodeID string) (runEvent, bool) {
	for _, event := range events {
		if (event.Type == "node.failed" || event.Type == "node.postcondition_failed") && event.NodeID == nodeID {
			return event, true
		}
	}
	return runEvent{}, false
}

func newHangAdapter(t *testing.T, shim string) *pi.Adapter {
	t.Helper()

	return pi.New(pi.Options{
		Binary:         shim,
		CacheDir:       t.TempDir(),
		StartupTimeout: 10 * time.Second,
		CommandTimeout: 2 * time.Second,
		ShutdownGrace:  200 * time.Millisecond,
	})
}

// waitForNodeRunning blocks until the engine has started the named node.
func waitForNodeRunning(t *testing.T, server *httptest.Server, project, runID, nodeID string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run := getRun(t, server, project, runID)
		if run.Status.Terminal() {
			t.Fatalf("run ended %s before node %s started: %s", run.Status, nodeID, run.Error)
		}
		if node := run.Node(nodeID); node != nil && node.Status == orchestrator.NodeRunning {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("node %s never started", nodeID)
}

// TestRung5Cancel proves a run cancellation stops a live harness, reports a
// cancellation rather than a failure, leaves no process, and can be resumed.
func TestRung5Cancel(t *testing.T) {
	requireE2E(t)

	shim, pidfile := writePiShim(t, "", shimHang)
	adapter := newHangAdapter(t, shim)
	workflow := `
id: cancel
defaults: {harness: pi}
nodes:
  - id: hang
    kind: agent
    prompt: wait forever
`
	server, _, project := newServer(t, "pi", adapter, map[string]string{"cancel": workflow})

	run := createRun(t, server, project, "cancel", "cancel the long node")
	waitForNodeRunning(t, server, project, run.ID, "hang")

	status, body := postJSON(t, runURL(server.URL, project, run.ID, "/cancel"), map[string]string{})
	if status != http.StatusOK {
		t.Fatalf("cancel: %d %s", status, body)
	}
	done := waitRun(t, server, project, run.ID, 30*time.Second)
	if done.Status != orchestrator.RunCancelled {
		t.Fatalf("run status = %s, want cancelled (%s)", done.Status, done.Error)
	}
	if node := done.Node("hang"); node.Status != orchestrator.NodeCancelled {
		t.Fatalf("node = %s, want cancelled", node.Status)
	}
	assertProcessGone(t, readPID(t, pidfile))

	status, body = postJSON(t, runURL(server.URL, project, run.ID, "/resume"), map[string]string{})
	if status != http.StatusOK {
		t.Fatalf("resume: %d %s", status, body)
	}
	waitForNodeRunning(t, server, project, run.ID, "hang")

	status, body = postJSON(t, runURL(server.URL, project, run.ID, "/cancel"), map[string]string{})
	if status != http.StatusOK {
		t.Fatalf("second cancel: %d %s", status, body)
	}
	done = waitRun(t, server, project, run.ID, 30*time.Second)
	if done.Status != orchestrator.RunCancelled {
		t.Fatalf("run status after resume = %s, want cancelled", done.Status)
	}
	assertProcessGone(t, readPID(t, pidfile))
}

// TestRung5Timeout proves a node deadline stops the harness and is reported as
// a retryable timeout, distinct from a cancellation, with no orphan.
func TestRung5Timeout(t *testing.T) {
	requireE2E(t)

	shim, pidfile := writePiShim(t, "", shimHang)
	adapter := newHangAdapter(t, shim)
	workflow := `
id: timeout
defaults: {harness: pi}
nodes:
  - id: hang
    kind: agent
    prompt: wait forever
    timeout: 1s
`
	server, _, project := newServer(t, "pi", adapter, map[string]string{"timeout": workflow})

	run := createRun(t, server, project, "timeout", "time the long node out")
	events := collectEvents(t, server, project, run.ID)
	done := waitRun(t, server, project, run.ID, 30*time.Second)

	if done.Status != orchestrator.RunFailed {
		t.Fatalf("run status = %s, want failed (%s)", done.Status, done.Error)
	}
	node := done.Node("hang")
	if node.FailureClass != string(harness.FailureTimeout) {
		t.Fatalf("failure class = %q, want timeout (%s)", node.FailureClass, node.Error)
	}
	failed, ok := failureEvent(events, "hang")
	if !ok {
		t.Fatal("missing node.failed event")
	}
	if retryable, _ := failed.Data["retryable"].(bool); !retryable {
		t.Fatalf("a timeout was reported non-retryable: %+v", failed.Data)
	}

	assertProcessGone(t, readPID(t, pidfile))
}

// TestRung5Crash proves an abrupt harness death is an unexpected-exit failure,
// leaves no orphan, and can be retried forward or resumed.
func TestRung5Crash(t *testing.T) {
	model, binary := requireRealPi(t)
	shim, pidfile := writePiShim(t, binary, shimCrash)
	adapter := newPiAdapter(t, shim)
	t.Setenv("PI_SHIM_CRASH_DELAY", "3")

	workflow := fmt.Sprintf(`
id: crash
defaults:
  harness: pi
  model: {id: %s, thinking: off}
nodes:
  - id: slow
    kind: agent
    skill: slow-bash
    prompt: |
      Call the bash tool with the command "sleep 8" now, then reply done.
    tools: [bash]
`, model)

	server, _, project := newServer(t, "pi", adapter, map[string]string{"crash": workflow})
	run := createRun(t, server, project, "crash", "crash the harness mid-run")
	events := collectEvents(t, server, project, run.ID)
	done := waitRun(t, server, project, run.ID, 40*time.Second)

	if done.Status != orchestrator.RunFailed {
		t.Fatalf("run status = %s, want failed (%s)", done.Status, done.Error)
	}
	node := done.Node("slow")
	if node.FailureClass != string(harness.FailureHarnessExit) {
		t.Fatalf("failure class = %q, want harness_exit (%s)", node.FailureClass, node.Error)
	}
	failed, ok := failureEvent(events, "slow")
	if !ok {
		t.Fatal("missing node.failed event")
	}
	if retryable, _ := failed.Data["retryable"].(bool); !retryable {
		t.Fatalf("an unexpected exit was reported non-retryable: %+v", failed.Data)
	}
	assertProcessGone(t, readPID(t, pidfile))

	// Retry forward re-executes the node; cancel before the next crash.
	status, body := postJSON(t, runURL(server.URL, project, run.ID, "/retry"), map[string]string{
		"nodeId": "slow",
		"mode":   "forward",
	})
	if status != http.StatusOK {
		t.Fatalf("retry: %d %s", status, body)
	}
	waitForNodeRunning(t, server, project, run.ID, "slow")
	if status, body := postJSON(t, runURL(server.URL, project, run.ID, "/cancel"), map[string]string{}); status != http.StatusOK {
		t.Fatalf("cancel after retry: %d %s", status, body)
	}
	cancelled := waitRun(t, server, project, run.ID, 30*time.Second)
	if cancelled.Status != orchestrator.RunCancelled {
		t.Fatalf("run status after retry = %s, want cancelled", cancelled.Status)
	}
	if attempts := cancelled.Node("slow").Attempts; attempts < 2 {
		t.Fatalf("attempts after retry = %d, want at least 2", attempts)
	}

	// Resume re-executes from the first unfinished node; cancel again.
	status, body = postJSON(t, runURL(server.URL, project, run.ID, "/resume"), map[string]string{})
	if status != http.StatusOK {
		t.Fatalf("resume: %d %s", status, body)
	}
	waitForNodeRunning(t, server, project, run.ID, "slow")
	if status, body := postJSON(t, runURL(server.URL, project, run.ID, "/cancel"), map[string]string{}); status != http.StatusOK {
		t.Fatalf("cancel after resume: %d %s", status, body)
	}
	resumed := waitRun(t, server, project, run.ID, 30*time.Second)
	if resumed.Status != orchestrator.RunCancelled {
		t.Fatalf("run status after resume = %s, want cancelled", resumed.Status)
	}
	if attempts := resumed.Node("slow").Attempts; attempts < 3 {
		t.Fatalf("attempts after resume = %d, want at least 3", attempts)
	}
	assertProcessGone(t, readPID(t, pidfile))
}

// TestRung5Postcondition proves a completed harness whose repository state is
// wrong fails as a postcondition, not a harness failure, and is never retried
// automatically even when retries are configured.
func TestRung5Postcondition(t *testing.T) {
	model, binary := requireRealPi(t)
	shim, pidfile := writePiShim(t, binary, shimPass)
	adapter := newPiAdapter(t, shim)

	workflow := fmt.Sprintf(`
id: postfail
defaults:
  harness: pi
  model: {id: %s, thinking: off}
nodes:
  - id: write
    kind: agent
    skill: write-file
    prompt: |
      Call the write tool now with path exactly "proof.txt" (lowercase, .txt)
      and content exactly "written by pi". Then reply done.
    tools: [write]
    retry: 3
    postcondition:
      type: file_exists
      path: missing.txt
`, model)

	server, _, project := newServer(t, "pi", adapter, map[string]string{"postfail": workflow})
	run := createRun(t, server, project, "postfail", "fail the postcondition")
	events := collectEvents(t, server, project, run.ID)
	done := waitRun(t, server, project, run.ID, 60*time.Second)

	if done.Status != orchestrator.RunFailed {
		t.Fatalf("run status = %s, want failed (%s)", done.Status, done.Error)
	}
	node := done.Node("write")
	if node.FailureClass != "postcondition" {
		t.Fatalf("failure class = %q, want postcondition (%s)", node.FailureClass, node.Error)
	}
	if node.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1: a postcondition failure must not retry automatically", node.Attempts)
	}

	failed, ok := failureEvent(events, "write")
	if !ok {
		t.Fatal("missing node.postcondition_failed event")
	}
	if failed.Type != "node.postcondition_failed" {
		t.Fatalf("event type = %q, want node.postcondition_failed", failed.Type)
	}
	if retryable, _ := failed.Data["retryable"].(bool); retryable {
		t.Fatalf("a postcondition failure was reported retryable: %+v", failed.Data)
	}

	// The checkpoint exists: the harness succeeded and its work is on the
	// branch, which is precisely why this is a repository failure.
	if node.Checkpoint == "" {
		t.Fatal("no checkpoint was recorded before the postcondition failed")
	}
	if body := gitShow(t, project, node.Checkpoint, "proof.txt"); strings.TrimSpace(body) == "" {
		t.Fatal("the checkpoint does not contain the harness's file")
	}

	assertProcessGone(t, readPID(t, pidfile))
}
