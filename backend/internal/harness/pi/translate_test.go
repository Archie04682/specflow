package pi

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"example.com/specflow/backend/internal/harness"
)

// recordingHost is the test host used by translation and adapter tests.
type recordingHost struct {
	mu         sync.Mutex
	events     []harness.Event
	asks       []harness.Ask
	answer     harness.Answer
	askErr     error
	injections chan harness.Injection
}

func (h *recordingHost) Emit(event harness.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	return nil
}

func (h *recordingHost) Ask(_ context.Context, ask harness.Ask) (harness.Answer, error) {
	h.mu.Lock()
	h.asks = append(h.asks, ask)
	h.mu.Unlock()
	return h.answer, h.askErr
}

func (h *recordingHost) asked() []harness.Ask {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]harness.Ask(nil), h.asks...)
}

func (h *recordingHost) Injections() <-chan harness.Injection {
	return h.injections
}

func (h *recordingHost) recorded() []harness.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]harness.Event(nil), h.events...)
}

func (h *recordingHost) kinds() []harness.Kind {
	events := h.recorded()
	kinds := make([]harness.Kind, len(events))
	for i, event := range events {
		kinds[i] = event.Kind
	}
	return kinds
}

func mustRecord(t *testing.T, line string) record {
	t.Helper()

	var rec record
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("decode fixture %q: %v", line, err)
	}
	return rec
}

func feed(t *testing.T, tr *translator, lines ...string) {
	t.Helper()

	for _, line := range lines {
		if err := tr.handle(mustRecord(t, line)); err != nil {
			t.Fatalf("handle %q: %v", line, err)
		}
	}
}

func assertKinds(t *testing.T, got, want []harness.Kind) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kind %d = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestTranslateTextStream(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"turn_start"}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hel"}}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"lo"}}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_end","contentIndex":0,"content":"Hello"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]}}`,
		`{"type":"turn_end"}`,
	)

	assertKinds(t, host.kinds(), []harness.Kind{
		harness.KindTurnStarted,
		harness.KindTextDelta,
		harness.KindTextDelta,
		harness.KindTextDone,
		harness.KindTurnFinished,
	})

	events := host.recorded()
	if events[1].Text != "Hel" || events[2].Text != "lo" {
		t.Fatalf("deltas = %q, %q, want Hel, lo", events[1].Text, events[2].Text)
	}
	if events[3].Text != "Hello" {
		t.Fatalf("text_done = %q, want the authoritative block", events[3].Text)
	}

	result := tr.result(harness.StatusCompleted)
	if result.Text != "Hello" {
		t.Fatalf("result text = %q, want Hello", result.Text)
	}
	if result.Turns != 1 {
		t.Fatalf("turns = %d, want 1", result.Turns)
	}
}

func TestTranslateThinkingStream(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"message_update","assistantMessageEvent":{"type":"thinking_start","contentIndex":0}}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","contentIndex":0,"delta":"hmm"}}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"thinking_end","contentIndex":0,"content":"hmm"}}`,
	)

	assertKinds(t, host.kinds(), []harness.Kind{
		harness.KindThinkingDelta,
		harness.KindThinkingDone,
	})
	if got := host.recorded()[1].Text; got != "hmm" {
		t.Fatalf("thinking_done = %q, want hmm", got)
	}
}

func TestTranslateToolCallsDoNotBecomeEvents(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"message_update","assistantMessageEvent":{"type":"toolcall_start","contentIndex":1,"id":"c1","toolName":"read"}}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"toolcall_end","contentIndex":1,"toolCall":{"id":"c1","name":"read"}}}`,
	)

	if got := host.kinds(); len(got) != 0 {
		t.Fatalf("kinds = %v, want none (tool intent is not execution)", got)
	}
}

func TestTranslateToolLifecycle(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"tool_execution_start","toolCallId":"c1","toolName":"read","args":{"path":"main.go"}}`,
		`{"type":"tool_execution_update","toolCallId":"c1","toolName":"read","partialResult":{"content":[{"type":"text","text":"package main"}]}}`,
		`{"type":"tool_execution_end","toolCallId":"c1","toolName":"read","result":{"content":[{"type":"text","text":"package main"}]},"isError":false,"durationMs":12}`,
	)

	assertKinds(t, host.kinds(), []harness.Kind{
		harness.KindToolStarted,
		harness.KindToolUpdated,
		harness.KindToolFinished,
	})

	events := host.recorded()
	if events[0].Tool.CallID != "c1" || events[0].Tool.Name != "read" {
		t.Fatalf("tool_started = %#v, want read with call id c1", events[0].Tool)
	}
	if events[1].Tool.Summary != "package main" {
		t.Fatalf("tool_updated summary = %q, want partial output", events[1].Tool.Summary)
	}
	if events[2].Tool.DurationMS != 12 {
		t.Fatalf("duration = %d, want 12", events[2].Tool.DurationMS)
	}

	result := tr.result(harness.StatusCompleted)
	if result.Tools != 1 {
		t.Fatalf("tool count = %d, want 1", result.Tools)
	}
	if len(result.Files) != 0 {
		t.Fatalf("files = %#v, want none for a read", result.Files)
	}
}

func TestTranslateWriteProducesFileChange(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"tool_execution_end","toolCallId":"c2","toolName":"write","args":{"path":"internal/auth.go"},"result":{"content":[{"type":"text","text":"wrote 42 lines"}]}}`,
	)

	assertKinds(t, host.kinds(), []harness.Kind{
		harness.KindToolFinished,
		harness.KindFileChanged,
	})

	change := host.recorded()[1].File
	if change == nil || change.Path != "internal/auth.go" || change.Tool != "write" {
		t.Fatalf("file change = %#v, want internal/auth.go via write", change)
	}
	if result := tr.result(harness.StatusCompleted); len(result.Files) != 1 {
		t.Fatalf("result files = %#v, want one entry", result.Files)
	}
}

func TestTranslateWriteUsesStartArgs(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	// Pi 1.5 sends the arguments on the start record and omits them from the
	// end record; the file attribution must survive that.
	feed(t, tr,
		`{"type":"tool_execution_start","toolCallId":"c4","toolName":"write","args":{"path":"proof.txt"}}`,
		`{"type":"tool_execution_end","toolCallId":"c4","toolName":"write","result":{"content":[{"type":"text","text":"wrote 1 line"}]}}`,
	)

	assertKinds(t, host.kinds(), []harness.Kind{
		harness.KindToolStarted,
		harness.KindToolFinished,
		harness.KindFileChanged,
	})

	change := host.recorded()[2].File
	if change == nil || change.Path != "proof.txt" || change.Tool != "write" {
		t.Fatalf("file change = %#v, want proof.txt via write", change)
	}
}

func TestTranslateFailedEditKeepsErrorFlag(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"tool_execution_end","toolCallId":"c3","toolName":"edit","args":{"path":"main.go"},"isError":true,"result":{"content":[{"type":"text","text":"oldText not found"}]}}`,
	)

	events := host.recorded()
	if len(events) != 2 {
		t.Fatalf("events = %v, want tool_finished and file_changed", events)
	}
	if !events[0].Tool.IsError {
		t.Fatal("tool_finished did not carry isError")
	}
	if events[1].File == nil || events[1].File.Path != "main.go" {
		t.Fatalf("file change = %#v, want main.go", events[1].File)
	}
}

func TestTranslateUsageAtTurnEnd(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"message_update","usage":{"input":100,"output":5,"cacheRead":2,"cacheWrite":1,"totalTokens":108,"cost":{"total":0.0125}},"assistantMessageEvent":{"type":"text_delta","delta":"hi"}}`,
		`{"type":"turn_end"}`,
	)

	events := host.recorded()
	usageEvents := 0
	for _, event := range events {
		if event.Kind != harness.KindUsage {
			continue
		}
		usageEvents++
		usage := event.Usage
		if usage == nil {
			t.Fatal("usage event has no payload")
		}
		if usage.InputTokens != 100 || usage.OutputTokens != 5 || usage.TotalTokens != 108 {
			t.Fatalf("usage = %#v, want the cumulative values", usage)
		}
		if usage.CostUSD != 0.0125 {
			t.Fatalf("cost = %v, want 0.0125", usage.CostUSD)
		}
	}
	if usageEvents != 1 {
		t.Fatalf("usage events = %d, want exactly one per turn", usageEvents)
	}

	if result := tr.result(harness.StatusCompleted); result.Usage.TotalTokens != 108 {
		t.Fatalf("result usage = %#v, want the cumulative values", result.Usage)
	}
}

func TestTranslateAgentEndDoesNotSettle(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"agent_start"}`,
		`{"type":"agent_end","messages":[],"willRetry":false}`,
	)

	if tr.settled {
		t.Fatal("agent_end settled the execution; only agent_settled may do that")
	}
	if got := host.kinds(); len(got) != 1 || got[0] != harness.KindNotice {
		t.Fatalf("kinds = %v, want one notice", got)
	}

	feed(t, tr, `{"type":"agent_settled","aborted":false}`)
	if !tr.settled || tr.aborted {
		t.Fatalf("settled = %v, aborted = %v, want settled and not aborted", tr.settled, tr.aborted)
	}
}

func TestTranslateAbortedSettle(t *testing.T) {
	tr := newTranslator(&recordingHost{})

	feed(t, tr, `{"type":"agent_settled","aborted":true}`)

	if !tr.settled || !tr.aborted {
		t.Fatalf("settled = %v, aborted = %v, want both true", tr.settled, tr.aborted)
	}
}

func TestTranslateRecordsInBandError(t *testing.T) {
	tr := newTranslator(&recordingHost{})

	feed(t, tr,
		`{"type":"turn_start"}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"error","reason":"error","error":"rate limited"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"rate limited"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	if got := tr.executionError(); got != "rate limited" {
		t.Fatalf("execution error = %q, want the provider message", got)
	}
	if !tr.settled {
		t.Fatal("settle marker was not recorded")
	}
}

func TestTranslateRetryClearsInBandError(t *testing.T) {
	tr := newTranslator(&recordingHost{})

	feed(t, tr,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"rate limited"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_end","messages":[],"willRetry":true}`,
	)

	if got := tr.executionError(); got == "" {
		t.Fatal("a retry announcement cleared the error before the retry succeeded")
	}

	feed(t, tr,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"stopReason":"stop"}}`,
		`{"type":"turn_end"}`,
		`{"type":"agent_settled","aborted":false}`,
	)

	if got := tr.executionError(); got != "" {
		t.Fatalf("execution error = %q, want cleared by the successful retry", got)
	}
}

func TestTranslateSuccessfulMessageClearsInBandError(t *testing.T) {
	tr := newTranslator(&recordingHost{})

	feed(t, tr,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"overloaded"}}`,
		`{"type":"turn_end"}`,
		`{"type":"turn_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"stopReason":"stop"}}`,
		`{"type":"turn_end"}`,
	)

	if got := tr.executionError(); got != "" {
		t.Fatalf("execution error = %q, want cleared by the successful message", got)
	}
}

func TestTranslateCompactionAndRetryNotices(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	feed(t, tr,
		`{"type":"compaction_start","reason":"threshold"}`,
		`{"type":"compaction_end","reason":"threshold","aborted":false}`,
		`{"type":"auto_retry_start","attempt":2}`,
	)

	codes := []string{}
	for _, event := range host.recorded() {
		if event.Notice != nil {
			codes = append(codes, event.Notice.Code)
		}
	}
	want := []string{"compaction", "compaction", "retry"}
	if len(codes) != len(want) {
		t.Fatalf("notice codes = %v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("notice codes = %v, want %v", codes, want)
		}
	}
}

func TestTranslateUIDialogs(t *testing.T) {
	cases := []struct {
		name string
		line string
		want harness.Ask
	}{
		{
			name: "select",
			line: `{"type":"extension_ui_request","id":"q1","method":"select","title":"Pick","options":["a","b"],"timeout":5000}`,
			want: harness.Ask{ID: "q1", Kind: harness.AskSelect, Title: "Pick", Options: []string{"a", "b"}, Timeout: 5 * time.Second},
		},
		{
			name: "confirm",
			line: `{"type":"extension_ui_request","id":"q2","method":"confirm","title":"Sure?","message":"All messages will be lost.","timeout":1000}`,
			want: harness.Ask{ID: "q2", Kind: harness.AskConfirm, Title: "Sure?", Message: "All messages will be lost.", Timeout: time.Second},
		},
		{
			name: "input",
			line: `{"type":"extension_ui_request","id":"q3","method":"input","title":"Name","placeholder":"type..."}`,
			want: harness.Ask{ID: "q3", Kind: harness.AskInput, Title: "Name", Placeholder: "type..."},
		},
		{
			name: "editor",
			line: `{"type":"extension_ui_request","id":"q4","method":"editor","title":"Edit","prefill":"line 1"}`,
			want: harness.Ask{ID: "q4", Kind: harness.AskEditor, Title: "Edit", Prefill: "line 1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := &recordingHost{}
			tr := newTranslator(host)

			ask, isDialog, err := tr.handleUI(mustRecord(t, tc.line))
			if err != nil {
				t.Fatalf("handleUI: %v", err)
			}
			if !isDialog {
				t.Fatal("dialog was not recognized as a question")
			}
			if ask.ID != tc.want.ID || ask.Kind != tc.want.Kind || ask.Title != tc.want.Title ||
				ask.Message != tc.want.Message || ask.Placeholder != tc.want.Placeholder ||
				ask.Prefill != tc.want.Prefill || ask.Timeout != tc.want.Timeout {
				t.Fatalf("ask = %#v, want %#v", ask, tc.want)
			}
			if len(tc.want.Options) > 0 {
				if len(ask.Options) != len(tc.want.Options) {
					t.Fatalf("options = %v, want %v", ask.Options, tc.want.Options)
				}
				for i := range tc.want.Options {
					if ask.Options[i] != tc.want.Options[i] {
						t.Fatalf("options = %v, want %v", ask.Options, tc.want.Options)
					}
				}
			}
		})
	}
}

func TestTranslateUIFireAndForget(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	for _, line := range []string{
		`{"type":"extension_ui_request","id":"n1","method":"notify","message":"careful","notifyType":"warning"}`,
		`{"type":"extension_ui_request","id":"n2","method":"setStatus","statusKey":"k","statusText":"working"}`,
		`{"type":"extension_ui_request","id":"n3","method":"setWidget","widgetLines":["x"]}`,
	} {
		if _, isDialog, err := tr.handleUI(mustRecord(t, line)); err != nil || isDialog {
			t.Fatalf("handleUI(%s) = dialog %v, err %v; want neither", line, isDialog, err)
		}
	}

	kinds := host.kinds()
	if len(kinds) != 2 {
		t.Fatalf("kinds = %v, want a notice and a status only", kinds)
	}
	if kinds[0] != harness.KindNotice || kinds[1] != harness.KindStatus {
		t.Fatalf("kinds = %v, want notice then status", kinds)
	}

	notice := host.recorded()[0].Notice
	if notice == nil || notice.Level != harness.NoticeWarning {
		t.Fatalf("notice = %#v, want a warning", notice)
	}
	if got := host.recorded()[1].Text; got != "working" {
		t.Fatalf("status text = %q, want working", got)
	}
}

func TestTranslateSwallowsBridgeMarker(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	line := `{"type":"extension_ui_request","id":"m1","method":"notify","message":"` + BridgeMarker + `","notifyType":"info"}`
	ask, isDialog, err := tr.handleUI(mustRecord(t, line))
	if err != nil {
		t.Fatalf("handleUI: %v", err)
	}
	if isDialog || ask.ID != "" {
		t.Fatalf("bridge marker became a question: %#v", ask)
	}
	if kinds := host.kinds(); len(kinds) != 0 {
		t.Fatalf("kinds = %v, want no event for the handshake", kinds)
	}
}

func TestTranslateUnknownRecordIsReportedOnce(t *testing.T) {
	host := &recordingHost{}
	tr := newTranslator(host)

	for i := 0; i < 3; i++ {
		if err := tr.handle(mustRecord(t, `{"type":"brand_new_record"}`)); err != nil {
			t.Fatalf("handle: %v", err)
		}
	}

	kinds := host.kinds()
	if len(kinds) != 1 {
		t.Fatalf("kinds = %v, want exactly one notice after three identical unknown records", kinds)
	}
	notice := host.recorded()[0].Notice
	if notice == nil || notice.Code != "unknown_record" {
		t.Fatalf("notice = %#v, want an unknown_record notice", notice)
	}
}

// askStart renders a specflow_ask tool start carrying the arguments Pi emits
// before the dialog request. The host records the event; only the arguments
// matter to the dialog merge.
func askStart(t *testing.T, callID, title, question, kind string, options ...string) string {
	t.Helper()

	args := map[string]any{"question": question, "title": title, "kind": kind}
	if len(options) > 0 {
		args["options"] = options
	}
	encoded, err := json.Marshal(map[string]any{
		"type":       "tool_execution_start",
		"toolCallId": callID,
		"toolName":   askToolName,
		"args":       args,
	})
	if err != nil {
		t.Fatalf("encode specflow_ask start: %v", err)
	}
	return string(encoded)
}

func TestTranslateAskGainsQuestionDescription(t *testing.T) {
	const question = "Which provider authenticates the gateway?"

	cases := []struct {
		name   string
		start  string
		dialog string
		want   harness.Ask
	}{
		{
			name:   "select",
			start:  askStart(t, "c1", "Which auth?", question, "select", "OAuth", "Password"),
			dialog: `{"type":"extension_ui_request","id":"q1","method":"select","title":"Which auth?","options":["OAuth","Password"]}`,
			want: harness.Ask{
				ID: "q1", Kind: harness.AskSelect, Title: "Which auth?",
				Message: question, Options: []string{"OAuth", "Password"},
			},
		},
		{
			name:   "input",
			start:  askStart(t, "c2", "Service name", question, "input"),
			dialog: `{"type":"extension_ui_request","id":"q2","method":"input","title":"Service name","placeholder":"type..."}`,
			want: harness.Ask{
				ID: "q2", Kind: harness.AskInput, Title: "Service name",
				Message: question, Placeholder: "type...",
			},
		},
		{
			name:   "editor",
			start:  askStart(t, "c3", "Edit the plan", question, "editor"),
			dialog: `{"type":"extension_ui_request","id":"q3","method":"editor","title":"Edit the plan","prefill":"draft"}`,
			want: harness.Ask{
				ID: "q3", Kind: harness.AskEditor, Title: "Edit the plan",
				Message: question, Prefill: "draft",
			},
		},
		{
			name:   "confirm keeps its own message",
			start:  askStart(t, "c4", "Overwrite config", "Should I overwrite the existing config?", "confirm"),
			dialog: `{"type":"extension_ui_request","id":"q4","method":"confirm","title":"Overwrite config","message":"All messages will be lost."}`,
			want: harness.Ask{
				ID: "q4", Kind: harness.AskConfirm, Title: "Overwrite config",
				Message: "All messages will be lost.",
			},
		},
		{
			name:   "dialog missing options is filled from the tool call",
			start:  askStart(t, "c5", "Pick one", question, "select", "alpha", "beta"),
			dialog: `{"type":"extension_ui_request","id":"q5","method":"select","title":"Pick one"}`,
			want: harness.Ask{
				ID: "q5", Kind: harness.AskSelect, Title: "Pick one",
				Message: question, Options: []string{"alpha", "beta"},
			},
		},
		{
			name:   "unmatched dialog is unchanged",
			dialog: `{"type":"extension_ui_request","id":"q6","method":"select","title":"Pick","options":["a","b"]}`,
			want: harness.Ask{
				ID: "q6", Kind: harness.AskSelect, Title: "Pick", Options: []string{"a", "b"},
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			tr := newTranslator(&recordingHost{})
			if test.start != "" {
				feed(t, tr, test.start)
			}

			ask, isDialog, err := tr.handleUI(mustRecord(t, test.dialog))
			if err != nil {
				t.Fatalf("handleUI: %v", err)
			}
			if !isDialog {
				t.Fatal("dialog was not recognized as a question")
			}
			if !reflect.DeepEqual(ask, test.want) {
				t.Fatalf("ask = %#v, want %#v", ask, test.want)
			}
		})
	}
}

func TestTranslateAskMatchesByTitleThenOldest(t *testing.T) {
	tr := newTranslator(&recordingHost{})
	feed(t, tr,
		askStart(t, "c1", "First", "first question", "input"),
		askStart(t, "c2", "Second", "second question", "input"),
	)

	// A title match consumes the second call without disturbing the FIFO.
	ask, _, err := tr.handleUI(mustRecord(t, `{"type":"extension_ui_request","id":"q1","method":"input","title":"Second"}`))
	if err != nil {
		t.Fatalf("handleUI: %v", err)
	}
	if ask.Message != "second question" {
		t.Fatalf("message = %q, want the matching call's question", ask.Message)
	}

	// With no matching title the oldest remaining call wins.
	ask, _, err = tr.handleUI(mustRecord(t, `{"type":"extension_ui_request","id":"q2","method":"input","title":"Unknown"}`))
	if err != nil {
		t.Fatalf("handleUI: %v", err)
	}
	if ask.Message != "first question" {
		t.Fatalf("message = %q, want the oldest call's question", ask.Message)
	}
	if len(tr.pendingAsks) != 0 {
		t.Fatalf("pending asks = %d after both dialogs, want none", len(tr.pendingAsks))
	}

	// A dialog with nothing pending keeps its own fields.
	ask, isDialog, err := tr.handleUI(mustRecord(t, `{"type":"extension_ui_request","id":"q3","method":"input","title":"Third"}`))
	if err != nil || !isDialog {
		t.Fatalf("unmatched dialog = dialog %v, err %v", isDialog, err)
	}
	if ask.Message != "" || ask.Title != "Third" {
		t.Fatalf("unmatched ask = %#v, want the dialog's own fields", ask)
	}
}

func TestTranslateAskDroppedWhenToolEnds(t *testing.T) {
	tr := newTranslator(&recordingHost{})
	feed(t, tr,
		askStart(t, "c1", "Which auth?", "Which provider authenticates the gateway?", "select", "OAuth"),
		`{"type":"tool_execution_end","toolCallId":"c1","toolName":"specflow_ask","result":{"content":[{"type":"text","text":"OAuth"}]}}`,
	)

	ask, isDialog, err := tr.handleUI(mustRecord(t, `{"type":"extension_ui_request","id":"q1","method":"select","title":"Which auth?","options":["OAuth"]}`))
	if err != nil || !isDialog {
		t.Fatalf("dialog = %v, err = %v", isDialog, err)
	}
	if ask.Message != "" {
		t.Fatalf("message = %q, want no description after the tool call ended", ask.Message)
	}
	if len(tr.pendingAsks) != 0 {
		t.Fatalf("pending asks = %d, want none", len(tr.pendingAsks))
	}
}

func TestTranslateAskIgnoresEmptyQuestion(t *testing.T) {
	tr := newTranslator(&recordingHost{})
	feed(t, tr, askStart(t, "c1", "No question", "", "input"))

	ask, _, err := tr.handleUI(mustRecord(t, `{"type":"extension_ui_request","id":"q1","method":"input","title":"No question"}`))
	if err != nil {
		t.Fatalf("handleUI: %v", err)
	}
	if ask.Message != "" {
		t.Fatalf("message = %q, want none for an empty question argument", ask.Message)
	}
}

// TestTranslateAskZeroValueTranslatorIsSafe pins that the merge survives a
// translator constructed directly rather than through newTranslator.
func TestTranslateAskZeroValueTranslatorIsSafe(t *testing.T) {
	var tr translator

	ask, isDialog, err := tr.handleUI(mustRecord(t, `{"type":"extension_ui_request","id":"q1","method":"input","title":"Name"}`))
	if err != nil {
		t.Fatalf("handleUI: %v", err)
	}
	if !isDialog {
		t.Fatal("dialog was not recognized")
	}
	if ask.Title != "Name" || ask.Message != "" {
		t.Fatalf("ask = %#v, want the dialog's own fields", ask)
	}
}

func TestTranslateToolSummaryBound(t *testing.T) {
	cases := []struct {
		name      string
		length    int
		wantLen   int
		truncated bool
	}{
		{name: "above the old two kilobyte bound", length: 6000, wantLen: 6000},
		{name: "at the summary bound", length: toolSummaryLimit, wantLen: toolSummaryLimit},
		{name: "over the summary bound", length: toolSummaryLimit + 500, wantLen: toolSummaryLimit + 3, truncated: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			host := &recordingHost{}
			tr := newTranslator(host)
			result := `{"type":"tool_execution_end","toolCallId":"c1","toolName":"bash","result":{"content":[{"type":"text","text":"` +
				strings.Repeat("x", test.length) + `"}]}}`
			feed(t, tr, result)

			events := host.recorded()
			if len(events) != 1 || events[0].Tool == nil {
				t.Fatalf("events = %#v, want one tool_finished", events)
			}
			summary := events[0].Tool.Summary
			if len(summary) != test.wantLen {
				t.Fatalf("summary length = %d, want %d", len(summary), test.wantLen)
			}
			if test.truncated {
				if !strings.HasSuffix(summary, "...") {
					t.Fatal("an over-bound summary was not marked truncated")
				}
			} else if summary != strings.Repeat("x", test.length) {
				t.Fatal("a long summary was cut before the bound")
			}
		})
	}
}

func TestTruncateRunesKeepsWholeCharacters(t *testing.T) {
	// Two-byte characters: an odd byte limit would split one without the guard.
	text := strings.Repeat("\u0436", 100)
	got := truncateRunes(text, 51)

	if len(got) > 51 {
		t.Fatalf("truncated length = %d, want at most 51", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatal("truncation produced invalid UTF-8")
	}
	if !strings.HasPrefix(text, got) {
		t.Fatal("truncation did not keep a prefix of the text")
	}
}