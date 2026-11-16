package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
)

var fakePi string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "specflow-fake-pi-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create fake pi dir: %v\n", err)
		os.Exit(1)
	}

	fakePi = filepath.Join(dir, "fake-pi")
	build := exec.Command("go", "build", "-o", fakePi, "./testdata/fake-pi")
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fake pi: %v\n%s\n", err, output)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func testAdapter(t *testing.T) *Adapter {
	t.Helper()

	return New(Options{
		Binary:         fakePi,
		MinVersion:     "1.0.0",
		CacheDir:       t.TempDir(),
		StartupTimeout: 2 * time.Second,
		CommandTimeout: 2 * time.Second,
		ShutdownGrace:  150 * time.Millisecond,
		BridgeConfirm:  500 * time.Millisecond,
	})
}

func writeScript(t *testing.T, lines ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "script.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

// skillDir writes a minimal Agent Skills directory. The adapter passes the
// path to Pi; fake-pi derives the registered command from the directory name.
func skillDir(t *testing.T, name string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	body := "---\nname: " + name + "\ndescription: fixture skill\n---\n\nDo the thing.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return dir
}

func fakeEnv(t *testing.T, pairs ...string) map[string]string {
	t.Helper()

	if len(pairs)%2 != 0 {
		t.Fatal("fakeEnv needs key/value pairs")
	}

	env := make(map[string]string, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		env[pairs[i]] = pairs[i+1]
	}
	return env
}

func runOnce(t *testing.T, adapter *Adapter, req harness.Request, host harness.Host) (harness.Result, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return adapter.Run(ctx, req, host)
}

func readCapture(t *testing.T, path string) []map[string]any {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}

	var records []map[string]any
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode captured line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func assertFailure(t *testing.T, err error, class harness.FailureClass) *harness.Failure {
	t.Helper()

	if err == nil {
		t.Fatalf("Run returned nil error, want a %s failure", class)
	}

	var failure *harness.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T, want *harness.Failure", err)
	}
	if failure.Class != class {
		t.Fatalf("class = %q, want %q (%v)", failure.Class, class, failure)
	}
	return failure
}

func TestIsRetryableCoversEveryClass(t *testing.T) {
	want := map[harness.FailureClass]bool{
		harness.FailureAdapterMissing: false,
		harness.FailureUnsupported:    false,
		harness.FailureConfig:         false,
		harness.FailureAuth:           false,
		harness.FailureProvider:       true,
		harness.FailureStart:          true,
		harness.FailureProtocol:       true,
		harness.FailureHarnessExit:    true,
		harness.FailureTimeout:        true,
		harness.FailureCancelled:      false,
	}

	for _, class := range harness.AllFailureClasses {
		expected, ok := want[class]
		if !ok {
			t.Fatalf("no retryability expectation for class %q", class)
		}
		if got := isRetryable(class); got != expected {
			t.Errorf("isRetryable(%q) = %t, want %t", class, got, expected)
		}
	}
}

func TestBuildArgs(t *testing.T) {
	bridge := "/tmp/specflow-bridge.ts"

	cases := []struct {
		name string
		req  harness.Request
		cfg  Config
		want []string
	}{
		{
			name: "default node",
			want: []string{"--mode", "rpc", "--no-session", "-e", bridge, "--tools", "+specflow_ask", "--no-approve"},
		},
		{
			name: "model and thinking",
			req:  harness.Request{Model: harness.Model{ID: "provider/model", Thinking: "high"}},
			want: []string{"--mode", "rpc", "--no-session", "-e", bridge, "--model", "provider/model", "--thinking", "high", "--tools", "+specflow_ask", "--no-approve"},
		},
		{
			name: "provider and offline",
			cfg:  Config{Provider: "deepseek", Offline: true},
			want: []string{"--mode", "rpc", "--no-session", "-e", bridge, "--provider", "deepseek", "--tools", "+specflow_ask", "--no-approve", "--offline"},
		},
		{
			name: "explicit tools and deny list",
			req:  harness.Request{Tools: harness.ToolPolicy{Allow: []string{"read", "bash"}, Deny: []string{"write"}}},
			want: []string{"--mode", "rpc", "--no-session", "-e", bridge, "--tools", "read,bash,specflow_ask", "--exclude-tools", "write", "--no-approve"},
		},
		{
			name: "bound skill",
			req:  harness.Request{Skill: &harness.Skill{Name: "openspec-explore", Paths: []string{"/skills/a", "/skills/b"}}},
			want: []string{"--mode", "rpc", "--no-session", "-e", bridge, "--tools", "+specflow_ask", "--no-skills", "--skill", "/skills/a", "--skill", "/skills/b", "--no-approve"},
		},
		{
			name: "trusted node",
			req:  harness.Request{Trust: true},
			want: []string{"--mode", "rpc", "--no-session", "-e", bridge, "--tools", "+specflow_ask", "--approve"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildArgs(tc.req, tc.cfg, bridge)
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPromptTextForcesBoundSkill(t *testing.T) {
	req := harness.Request{
		Prompt: "Map the repository",
		Skill:  &harness.Skill{Name: "openspec-explore", Paths: []string{"/skills/a"}},
	}

	want := "/skill:openspec-explore\n\nMap the repository"
	got, err := promptForRequest(req)
	if err != nil {
		t.Fatalf("promptForRequest: %v", err)
	}
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}

	plain := harness.Request{Prompt: "Review current changes"}
	got, err = promptForRequest(plain)
	if err != nil {
		t.Fatalf("promptForRequest plain: %v", err)
	}
	if got != "Review current changes" {
		t.Fatalf("prompt = %q, want the plain prompt", got)
	}
}

func TestRunHappyPath(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hel"}}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"lo"}}`,
		`{"type":"message_update","usage":{"input":10,"output":2,"totalTokens":12,"cost":{"total":0.001}},"assistantMessageEvent":{"type":"text_end","content":"Hello"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	host := &recordingHost{}
	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "smoke",
		Prompt: "say hello",
		Env:    fakeEnv(t, "FAKE_PI_SCRIPT", script),
	}, host)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Text != "Hello" {
		t.Fatalf("text = %q, want Hello", result.Text)
	}
	if result.Turns != 1 {
		t.Fatalf("turns = %d, want 1", result.Turns)
	}
	if result.Usage.TotalTokens != 12 {
		t.Fatalf("total tokens = %d, want 12", result.Usage.TotalTokens)
	}
	if result.Ref.Session != "fake-session" {
		t.Fatalf("session = %q, want fake-session", result.Ref.Session)
	}

	kinds := host.kinds()
	if len(kinds) < 4 {
		t.Fatalf("kinds = %v, want telemetry from the script", kinds)
	}
}

func TestRunProviderErrorFails(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"rate limited by provider"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	host := &recordingHost{}
	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "provider",
		Prompt: "do the work",
		Env:    fakeEnv(t, "FAKE_PI_SCRIPT", script),
	}, host)

	failure := assertFailure(t, err, harness.FailureProvider)
	if !failure.Retryable {
		t.Fatalf("provider failure is not retryable: %v", failure)
	}
	if !strings.Contains(failure.Detail, "rate limited") {
		t.Fatalf("detail = %q, want the provider message", failure.Detail)
	}
	if result.Status != harness.StatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
}

func TestRunAuthErrorIsNotRetryable(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"401 unauthorized: invalid API key"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "auth",
		Prompt: "do the work",
		Env:    fakeEnv(t, "FAKE_PI_SCRIPT", script),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureAuth)
	if failure.Retryable {
		t.Fatalf("authentication failure must not be retryable: %v", failure)
	}
	if result.Status != harness.StatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
}

func TestRunForbiddenErrorIsNotRetryable(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"403 Forbidden"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	_, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "forbidden",
		Prompt: "do the work",
		Env:    fakeEnv(t, "FAKE_PI_SCRIPT", script),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureAuth)
	if failure.Retryable {
		t.Fatalf("a forbidden response must not be retryable: %v", failure)
	}
}

func TestRunRecoveredErrorCompletes(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"overloaded"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_end","messages":[],"willRetry":true}`,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"stopReason":"stop"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "recovered",
		Prompt: "do the work",
		Env:    fakeEnv(t, "FAKE_PI_SCRIPT", script),
	}, &recordingHost{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed after a successful retry", result.Status)
	}
	if result.Text != "done" {
		t.Fatalf("text = %q, want done", result.Text)
	}
}

func TestRunPromptRejectionClassifiesCredentials(t *testing.T) {
	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "no-key",
		Prompt: "do the work",
		Env:    fakeEnv(t, "FAKE_PI_PROMPT_ERROR", "No API key found for the selected model."),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureAuth)
	if failure.Retryable {
		t.Fatalf("credential rejection must not be retryable: %v", failure)
	}
	if result.Status != harness.StatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
}

func TestRunPromptRejectionClassifiesInvalidRequest(t *testing.T) {
	_, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "bad-request",
		Prompt: "do the work",
		Env:    fakeEnv(t, "FAKE_PI_PROMPT_ERROR", "invalid request: bad model arguments"),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureConfig)
	if failure.Retryable {
		t.Fatalf("invalid request must not be retryable: %v", failure)
	}
}

func TestRunStartFailureClassifiesUnknownModel(t *testing.T) {
	_, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "bad-model",
		Prompt: "do the work",
		Env: fakeEnv(t,
			"FAKE_PI_STDERR", `Error: Model "provider/not-a-model" not found.`,
			"FAKE_PI_EXIT_AT_START", "1",
		),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureConfig)
	if failure.Retryable {
		t.Fatalf("unknown model must not be retryable: %v", failure)
	}
	if !strings.Contains(failure.Detail, "not found") {
		t.Fatalf("detail = %q, want the startup diagnostic", failure.Detail)
	}
}

func TestRunStartFailureDefaultsToRetryable(t *testing.T) {
	_, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "start-failed",
		Prompt: "do the work",
		Env: fakeEnv(t,
			"FAKE_PI_STDERR", "the runtime failed to initialize",
			"FAKE_PI_EXIT_AT_START", "1",
		),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureStart)
	if !failure.Retryable {
		t.Fatalf("a transient start failure should be retryable: %v", failure)
	}
}

func TestRunConfirmsBoundSkillLoaded(t *testing.T) {
	dir := skillDir(t, "demo-skill")
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"stopReason":"stop"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "skill",
		Prompt: "do it",
		Skill:  &harness.Skill{Name: "demo-skill", Paths: []string{dir}},
		Env:    fakeEnv(t, "FAKE_PI_SCRIPT", script),
	}, &recordingHost{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
}

func TestRunUnloadableSkillFailsBeforePrompt(t *testing.T) {
	dir := skillDir(t, "demo-skill")
	capture := filepath.Join(t.TempDir(), "capture.jsonl")
	script := writeScript(t,
		`{"type":"agent_settled","aborted":false}`,
	)

	_, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "skill",
		Prompt: "do it",
		Skill:  &harness.Skill{Name: "demo-skill", Paths: []string{dir}},
		Env: fakeEnv(t,
			"FAKE_PI_OMIT_SKILL", "1",
			"FAKE_PI_CAPTURE", capture,
			"FAKE_PI_SCRIPT", script,
		),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureConfig)
	if !strings.Contains(failure.Detail, "skill:demo-skill") {
		t.Fatalf("detail = %q, want the missing skill command", failure.Detail)
	}
	for _, command := range readCapture(t, capture) {
		if command["type"] == "prompt" {
			t.Fatalf("a prompt was sent despite the unloadable skill: %v", command)
		}
	}
}

func TestRunSkillCheckUnavailableWarnsAndContinues(t *testing.T) {
	dir := skillDir(t, "demo-skill")
	script := writeScript(t,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"stopReason":"stop"}}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	host := &recordingHost{}
	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "skill",
		Prompt: "do it",
		Skill:  &harness.Skill{Name: "demo-skill", Paths: []string{dir}},
		Env: fakeEnv(t,
			"FAKE_PI_NO_GET_COMMANDS", "1",
			"FAKE_PI_SCRIPT", script,
		),
	}, host)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	warned := false
	for _, event := range host.recorded() {
		if event.Notice != nil && event.Notice.Code == "skill_check_unavailable" {
			warned = true
		}
	}
	if !warned {
		t.Fatal("no warning was emitted for an unsupported get_commands")
	}
}

func TestRunDoesNotEmitBridgeMarker(t *testing.T) {
	script := writeScript(t,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"stopReason":"stop"}}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	host := &recordingHost{}
	if _, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "marker",
		Prompt: "do it",
		Env:    fakeEnv(t, "FAKE_PI_SCRIPT", script),
	}, host); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, event := range host.recorded() {
		if strings.Contains(event.Text, BridgeMarker) {
			t.Fatalf("bridge marker leaked as text: %#v", event)
		}
		if event.Notice != nil && strings.Contains(event.Notice.Message, BridgeMarker) {
			t.Fatalf("bridge marker leaked as a notice: %#v", event.Notice)
		}
	}
}

func TestRunExitBeforeSettle(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"partial"}}`,
	)

	host := &recordingHost{}
	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "crash",
		Prompt: "work",
		Env: fakeEnv(t,
			"FAKE_PI_SCRIPT", script,
			"FAKE_PI_EXIT_BEFORE_SETTLE", "1",
			"FAKE_PI_STDERR", "boom: provider disconnected",
		),
	}, host)

	failure := assertFailure(t, err, harness.FailureHarnessExit)
	if !failure.Retryable {
		t.Fatal("harness exit should be retryable")
	}
	if !strings.Contains(failure.Detail, "boom: provider disconnected") {
		t.Fatalf("detail = %q, want the captured stderr", failure.Detail)
	}
	if result.Status != harness.StatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.Text != "partial" {
		t.Fatalf("text = %q, want the partial output", result.Text)
	}
}

func TestRunMalformedRecord(t *testing.T) {
	host := &recordingHost{}
	_, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "garbled",
		Prompt: "work",
		Env:    fakeEnv(t, "FAKE_PI_BAD_RECORD", "1"),
	}, host)

	assertFailure(t, err, harness.FailureProtocol)
}

func TestRunHandledPrompt(t *testing.T) {
	host := &recordingHost{}
	_, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "handled",
		Prompt: "work",
		Env:    fakeEnv(t, "FAKE_PI_HANDLED_PROMPT", "1"),
	}, host)

	failure := assertFailure(t, err, harness.FailureProtocol)
	if !strings.Contains(failure.Detail, "handled") {
		t.Fatalf("detail = %q, want the handled disposition explained", failure.Detail)
	}
}

func TestRunHandshakeTimeout(t *testing.T) {
	adapter := New(Options{
		Binary:         fakePi,
		CacheDir:       t.TempDir(),
		StartupTimeout: 200 * time.Millisecond,
		CommandTimeout: time.Second,
		ShutdownGrace:  100 * time.Millisecond,
	})

	host := &recordingHost{}
	_, err := runOnce(t, adapter, harness.Request{
		NodeID: "silent",
		Prompt: "work",
		Env:    fakeEnv(t, "FAKE_PI_IGNORE_GET_STATE", "1"),
	}, host)

	assertFailure(t, err, harness.FailureStart)
}

func TestRunShutdownEscalation(t *testing.T) {
	script := writeScript(t, `{"type":"agent_settled","aborted":false}`)
	pidFile := filepath.Join(t.TempDir(), "pid")

	adapter := testAdapter(t)
	started := time.Now()
	result, err := runOnce(t, adapter, harness.Request{
		NodeID: "stubborn",
		Prompt: "work",
		Env: fakeEnv(t,
			"FAKE_PI_SCRIPT", script,
			"FAKE_PI_IGNORE_STDIN_CLOSE", "1",
			"FAKE_PI_PIDFILE", pidFile,
		),
	}, &recordingHost{})
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed even though shutdown had to escalate", result.Status)
	}
	if elapsed < 2*adapter.opts.ShutdownGrace {
		t.Fatalf("Run took %v, want at least one grace period of escalation", elapsed)
	}

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
		t.Fatalf("parse pid %q: %v", raw, err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("process %d is still alive after Run (err = %v)", pid, err)
	}
}

func TestRunAskRoundTrip(t *testing.T) {
	// Pi's dialog protocol has no description slot: the question travels in
	// the specflow_ask tool's arguments, and Pi emits the tool start before the
	// dialog request. The adapter must surface it as the ask's message.
	const question = "Which provider authenticates the gateway?"
	script := writeScript(t,
		`{"type":"tool_execution_start","toolCallId":"c-ask","toolName":"specflow_ask","args":{"question":"`+question+`","title":"Which auth?","kind":"select","options":["OAuth","Password"]}}`,
		`{"type":"extension_ui_request","id":"q1","method":"select","title":"Which auth?","options":["OAuth","Password"]}`,
		`{"type":"tool_execution_end","toolCallId":"c-ask","toolName":"specflow_ask","result":{"content":[{"type":"text","text":"OAuth"}]}}`,
		`{"type":"turn_start"}`,
		`{"type":"agent_settled","aborted":false}`,
	)
	capture := filepath.Join(t.TempDir(), "capture.jsonl")

	host := &recordingHost{answer: harness.Answer{ID: "q1", Value: "OAuth"}}
	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "ask",
		Prompt: "work",
		Env: fakeEnv(t,
			"FAKE_PI_SCRIPT", script,
			"FAKE_PI_CAPTURE", capture,
		),
	}, host)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	asks := host.asked()
	if len(asks) != 1 {
		t.Fatalf("asked %d questions, want 1", len(asks))
	}
	if asks[0].Kind != harness.AskSelect || asks[0].Title != "Which auth?" {
		t.Fatalf("ask = %#v, want the select dialog", asks[0])
	}
	if asks[0].Message != question {
		t.Fatalf("ask message = %q, want the specflow_ask question", asks[0].Message)
	}
	if len(asks[0].Options) != 2 || asks[0].Options[0] != "OAuth" {
		t.Fatalf("options = %v, want the dialog options", asks[0].Options)
	}

	responded := false
	for _, record := range readCapture(t, capture) {
		if record["type"] != "extension_ui_response" {
			continue
		}
		responded = true
		if record["id"] != "q1" || record["value"] != "OAuth" {
			t.Fatalf("response = %#v, want the answered value", record)
		}
	}
	if !responded {
		t.Fatal("no extension_ui_response was written back to pi")
	}
}

// blockingAskHost waits for cancellation before answering, then returns a
// value that arrived too late to be used.
type blockingAskHost struct {
	recordingHost
	asked chan struct{}
	once  sync.Once
}

func (h *blockingAskHost) Ask(ctx context.Context, ask harness.Ask) (harness.Answer, error) {
	h.mu.Lock()
	h.asks = append(h.asks, ask)
	h.mu.Unlock()

	h.once.Do(func() { close(h.asked) })

	<-ctx.Done()
	return harness.Answer{ID: ask.ID, Value: "too late"}, nil
}

func TestRunCancelDuringAsk(t *testing.T) {
	script := writeScript(t,
		`{"type":"extension_ui_request","id":"q1","method":"input","title":"Which name?"}`,
		`{"type":"agent_settled","aborted":false}`,
	)
	capture := filepath.Join(t.TempDir(), "capture.jsonl")

	host := &blockingAskHost{asked: make(chan struct{})}
	adapter := testAdapter(t)

	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		result harness.Result
		err    error
	}
	done := make(chan outcome, 1)

	req := harness.Request{
		NodeID: "cancel-ask",
		Prompt: "work",
		Env: fakeEnv(t,
			"FAKE_PI_SCRIPT", script,
			"FAKE_PI_CAPTURE", capture,
		),
	}

	go func() {
		result, err := adapter.Run(ctx, req, host)
		done <- outcome{result: result, err: err}
	}()

	select {
	case <-host.asked:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("the adapter never asked the host")
	}

	cancel()

	select {
	case got := <-done:
		if got.result.Status != harness.StatusCancelled {
			t.Fatalf("status = %q, want cancelled", got.result.Status)
		}
		if got.err == nil {
			t.Fatal("Run returned nil error for a cancelled execution")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	var sawCancelled, sawAbort bool
	for _, record := range readCapture(t, capture) {
		if record["type"] == "extension_ui_response" && record["cancelled"] == true {
			sawCancelled = true
		}
		if record["type"] == "abort" {
			sawAbort = true
		}
	}
	if !sawCancelled {
		t.Fatal("the pending question was not cancelled for pi")
	}
	if !sawAbort {
		t.Fatal("no abort was sent to pi")
	}
}

// injectingHost injects a message the first time a given event kind arrives.
type injectingHost struct {
	recordingHost
	onKind    harness.Kind
	injection harness.Injection
	once      sync.Once
}

func (h *injectingHost) Emit(event harness.Event) error {
	if err := h.recordingHost.Emit(event); err != nil {
		return err
	}
	if event.Kind == h.onKind {
		h.once.Do(func() {
			h.injections <- h.injection
		})
	}
	return nil
}

func TestRunInjectionSteersWhileRunning(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"__wait_injection":true}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)
	capture := filepath.Join(t.TempDir(), "capture.jsonl")

	host := &injectingHost{
		recordingHost: recordingHost{injections: make(chan harness.Injection, 1)},
		onKind:        harness.KindTurnStarted,
		injection:     harness.Injection{ID: "i1", Text: "use PASETO"},
	}

	if _, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "inject",
		Prompt: "work",
		Env: fakeEnv(t,
			"FAKE_PI_SCRIPT", script,
			"FAKE_PI_CAPTURE", capture,
		),
	}, host); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var sawSteer, sawAck bool
	for _, record := range readCapture(t, capture) {
		if record["type"] == "steer" && record["message"] == "use PASETO" {
			sawSteer = true
		}
	}
	for _, event := range host.recorded() {
		if event.Kind == harness.KindInjectionAck && event.Injection != nil && event.Injection.ID == "i1" {
			sawAck = true
		}
	}
	if !sawSteer {
		t.Fatal("a message injected during a running turn was not sent as steer")
	}
	if !sawAck {
		t.Fatal("the injection was not acknowledged")
	}
}

func TestRunInjectionFollowsUpWhenIdle(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"turn_end"}`,
		`{"__wait_injection":true}`,
		`{"type":"agent_settled","aborted":false}`,
	)
	capture := filepath.Join(t.TempDir(), "capture.jsonl")

	host := &injectingHost{
		recordingHost: recordingHost{injections: make(chan harness.Injection, 1)},
		onKind:        harness.KindTurnFinished,
		injection:     harness.Injection{ID: "i2", Text: "also update the docs"},
	}

	if _, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "inject-idle",
		Prompt: "work",
		Env: fakeEnv(t,
			"FAKE_PI_SCRIPT", script,
			"FAKE_PI_CAPTURE", capture,
		),
	}, host); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var sawFollowUp bool
	for _, record := range readCapture(t, capture) {
		if record["type"] == "follow_up" && record["message"] == "also update the docs" {
			sawFollowUp = true
		}
		if record["type"] == "steer" {
			t.Fatal("an idle agent received a steer instead of a follow-up")
		}
	}
	if !sawFollowUp {
		t.Fatal("no follow_up command was sent")
	}
}

func TestProbeMissingBinary(t *testing.T) {
	adapter := New(Options{Binary: filepath.Join(t.TempDir(), "not-pi")})

	_, err := adapter.Probe(context.Background())

	failure := assertFailure(t, err, harness.FailureAdapterMissing)
	if failure.Retryable {
		t.Fatal("a missing executable must not be retryable")
	}
}

func TestProbeRejectsOldVersion(t *testing.T) {
	t.Setenv("FAKE_PI_VERSION", "0.9.0")

	adapter := New(Options{Binary: fakePi, CacheDir: t.TempDir(), MinVersion: "1.0.0"})
	_, err := adapter.Probe(context.Background())

	failure := assertFailure(t, err, harness.FailureConfig)
	if !strings.Contains(failure.Message, "1.0.0") {
		t.Fatalf("message = %q, want the minimum version named", failure.Message)
	}
}

func TestProbeReportsBridgeFailure(t *testing.T) {
	t.Setenv("FAKE_PI_BRIDGE_FAIL", "1")

	adapter := New(Options{Binary: fakePi, CacheDir: t.TempDir()})
	result, err := adapter.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	if result.Capabilities.Questions {
		t.Fatal("questions capability is true although the bridge failed to load")
	}
	if len(result.Warnings) == 0 {
		t.Fatal("no warning explains the downgrade")
	}
	if !strings.Contains(strings.Join(result.Warnings, " "), "failed to load") {
		t.Fatalf("warnings = %v, want the load failure named", result.Warnings)
	}
}

func TestProbeSuccess(t *testing.T) {
	adapter := New(Options{Binary: fakePi, CacheDir: t.TempDir()})

	result, err := adapter.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if result.RuntimeVersion != "1.2.3" {
		t.Fatalf("runtime version = %q, want 1.2.3", result.RuntimeVersion)
	}
	if !result.Capabilities.Questions {
		t.Fatalf("questions capability is false; warnings = %v", result.Warnings)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", result.Warnings)
	}
}

// signalHost closes a channel the first time a given event kind arrives.
type signalHost struct {
	recordingHost
	onKind harness.Kind
	seen   chan struct{}
	once   sync.Once
}

func (h *signalHost) Emit(event harness.Event) error {
	if err := h.recordingHost.Emit(event); err != nil {
		return err
	}
	if event.Kind == h.onKind {
		h.once.Do(func() { close(h.seen) })
	}
	return nil
}

func TestRunDeadlineIsTimeout(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"__sleep_ms":5000}`,
	)
	pidFile := filepath.Join(t.TempDir(), "pid")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	result, err := testAdapter(t).Run(ctx, harness.Request{
		NodeID: "deadline",
		Prompt: "work",
		Env: fakeEnv(t,
			"FAKE_PI_SCRIPT", script,
			"FAKE_PI_PIDFILE", pidFile,
		),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureTimeout)
	if !failure.Retryable {
		t.Fatalf("a timeout must be retryable: %v", failure)
	}
	if result.Status != harness.StatusTimedOut {
		t.Fatalf("status = %q, want timed_out", result.Status)
	}

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
		t.Fatalf("parse pid %q: %v", raw, err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("process %d is still alive after the deadline (err = %v)", pid, err)
	}
}

func TestRunCancelMidStreamLeavesNoProcess(t *testing.T) {
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"working"}}`,
		`{"__sleep_ms":5000}`,
	)
	pidFile := filepath.Join(t.TempDir(), "pid")

	host := &signalHost{onKind: harness.KindTurnStarted, seen: make(chan struct{})}
	adapter := testAdapter(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := harness.Request{
		NodeID: "cancel-stream",
		Prompt: "work",
		Env: fakeEnv(t,
			"FAKE_PI_SCRIPT", script,
			"FAKE_PI_PIDFILE", pidFile,
		),
	}

	type outcome struct {
		result harness.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := adapter.Run(ctx, req, host)
		done <- outcome{result: result, err: err}
	}()

	select {
	case <-host.seen:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("the execution never started a turn")
	}

	cancel()

	var got outcome
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	if got.result.Status != harness.StatusCancelled {
		t.Fatalf("status = %q, want cancelled", got.result.Status)
	}

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
		t.Fatalf("parse pid %q: %v", raw, err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("process %d is still alive after cancellation (err = %v)", pid, err)
	}
}

func TestClassifyErrorText(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		class     harness.FailureClass
		retryable bool
	}{
		{"missing key", "No API key found for the selected model.", harness.FailureAuth, false},
		{"unauthorized", "401 unauthorized: invalid API key", harness.FailureAuth, false},
		{"forbidden", "403 Forbidden", harness.FailureAuth, false},
		{"transient mentioning a key", "provider overloaded; the API key is valid", harness.FailureProvider, true},
		{"rate limited", "429 rate limited", harness.FailureProvider, true},
		{"unknown model", `Model "provider/not-a-model" not found.`, harness.FailureConfig, false},
		{"invalid request", "invalid request: bad arguments", harness.FailureConfig, false},
		{"generic provider error", "internal server error", harness.FailureProvider, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, retryable := classifyErrorText(tc.text)
			if class != tc.class || retryable != tc.retryable {
				t.Fatalf("classifyErrorText(%q) = %q/%t, want %q/%t", tc.text, class, retryable, tc.class, tc.retryable)
			}
		})
	}
}

func TestRunRetryWithoutSuccessFails(t *testing.T) {
	// Pi announces a retry, then settles without producing another assistant
	// message. The original error must not be treated as recovered.
	script := writeScript(t,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"overloaded"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_end","messages":[],"willRetry":true}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	result, err := runOnce(t, testAdapter(t), harness.Request{
		NodeID: "retry-noop",
		Prompt: "do the work",
		Env:    fakeEnv(t, "FAKE_PI_SCRIPT", script),
	}, &recordingHost{})

	failure := assertFailure(t, err, harness.FailureProvider)
	if !failure.Retryable {
		t.Fatalf("a provider failure should be retryable: %v", failure)
	}
	if result.Status != harness.StatusFailed {
		t.Fatalf("status = %q, want failed when the retry produced no message", result.Status)
	}
}

func TestPromptForRequestCombinesMultipleSkills(t *testing.T) {
	first := skillDir(t, "alpha-skill")
	second := skillDir(t, "beta-skill")
	req := harness.Request{
		Prompt: "Do the work",
		Skills: []harness.Skill{
			{Name: "alpha-skill", Paths: []string{first}},
			{Name: "beta-skill", Paths: []string{second}},
		},
	}

	prompt, err := promptForRequest(req)
	if err != nil {
		t.Fatalf("promptForRequest: %v", err)
	}
	for _, want := range []string{
		`<skill name="alpha-skill"`,
		`<skill name="beta-skill"`,
		"Do the thing.",
		"Do the work",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "/skill:") {
		t.Fatalf("multi-skill prompt still uses a single skill command:\n%s", prompt)
	}
}

func TestBuildArgsLoadsEverySkillPath(t *testing.T) {
	bridge := "/cache/specflow-bridge.ts"
	req := harness.Request{
		Skills: []harness.Skill{
			{Name: "alpha", Paths: []string{"/skills/alpha"}},
			{Name: "beta", Paths: []string{"/skills/beta"}},
		},
	}

	args := buildArgs(req, Config{}, bridge)
	joined := strings.Join(args, " ")
	for _, want := range []string{"--no-skills", "--skill /skills/alpha", "--skill /skills/beta"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %q", want, joined)
		}
	}
}
