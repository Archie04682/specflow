package harness

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"
)

func TestResultErr(t *testing.T) {
	for _, status := range AllStatuses {
		t.Run(string(status), func(t *testing.T) {
			result := Result{Status: status}

			err := result.Err()
			if status == StatusCompleted {
				if err != nil {
					t.Fatalf("Err() = %v, want nil for completed", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Err() = nil, want a failure for status %q", status)
			}

			var failure *Failure
			if !errors.As(err, &failure) {
				t.Fatalf("Err() = %T, want a *Failure", err)
			}
			if failure.Class == "" {
				t.Fatalf("failure class is empty for status %q", status)
			}
		})
	}
}

func TestResultErrPrefersRecordedFailure(t *testing.T) {
	recorded := &Failure{Class: FailureHarnessExit, Message: "pi exited", Retryable: true}

	err := Result{Status: StatusFailed, Failure: recorded}.Err()

	var failure *Failure
	if !errors.As(err, &failure) {
		t.Fatalf("Err() = %T, want a *Failure", err)
	}
	if failure != recorded {
		t.Fatalf("Err() = %v, want the recorded failure", failure)
	}
}

func TestFailureError(t *testing.T) {
	if got, want := (&Failure{Class: FailureConfig, Message: "bad flag"}).Error(), "config: bad flag"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if got, want := (&Failure{Class: FailureConfig, Message: "bad flag", Detail: "stderr"}).Error(), "config: bad flag: stderr"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestFailureClasses(t *testing.T) {
	seen := make(map[FailureClass]bool, len(AllFailureClasses))
	for _, class := range AllFailureClasses {
		if class == "" {
			t.Fatal("empty failure class")
		}
		if seen[class] {
			t.Fatalf("duplicate failure class %q", class)
		}
		seen[class] = true
	}
	if !seen[FailureProvider] {
		t.Fatalf("provider class is missing from AllFailureClasses")
	}
}

// TestContractJSONRoundTrip proves every message survives a JSON round trip,
// which is what keeps a future subprocess transport possible without changing
// the types.
func TestContractJSONRoundTrip(t *testing.T) {
	confirmed := true

	cases := []struct {
		name  string
		value any
	}{
		{"skill", &Skill{Name: "openspec-explore", Paths: []string{"/skills/explore"}}},
		{"model", Model{ID: "provider/model", Thinking: "high"}},
		{"tool-policy", ToolPolicy{Allow: []string{"read", "bash"}, Deny: []string{"write"}}},
		{
			"request",
			Request{
				NodeID:  "explore",
				Prompt:  "Map the repository",
				Skill:   &Skill{Name: "openspec-explore", Paths: []string{"/skills/explore"}},
				Model:   Model{ID: "provider/model", Thinking: "high"},
				Tools:   ToolPolicy{Allow: []string{"read"}, Deny: []string{"bash"}},
				Workdir: "/repo",
				Env:     map[string]string{"KEY": "value"},
				Trust:   true,
				Config:  json.RawMessage(`{"offline":true}`),
			},
		},
		{"usage", Usage{InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 4, TotalTokens: 10, CostUSD: 0.5}},
		{"file-change", FileChange{Path: "main.go", Tool: "write"}},
		{"harness-ref", HarnessRef{ID: "pi", Session: "s1", Version: "1.2.3"}},
		{"failure", &Failure{Class: FailureHarnessExit, Message: "exited", Detail: "stderr", Retryable: true}},
		{"notice", &Notice{Level: NoticeWarning, Code: "retry", Message: "retrying"}},
		{"injection-ack", &InjectionAck{ID: "i1", Disposition: "queued", Error: "late"}},
		{"tool-event", &ToolEvent{CallID: "c1", Name: "read", Args: map[string]any{"path": "main.go"}, Summary: "ok", IsError: true, DurationMS: 12}},
		{
			"result",
			Result{
				Status:  StatusFailed,
				Text:    "done",
				Turns:   2,
				Tools:   3,
				Usage:   Usage{InputTokens: 1, OutputTokens: 2},
				Files:   []FileChange{{Path: "main.go", Tool: "edit"}},
				Ref:     HarnessRef{ID: "pi"},
				Failure: &Failure{Class: FailureTimeout, Message: "slow"},
			},
		},
		{"ask", Ask{ID: "q1", Kind: AskSelect, Title: "t", Message: "m", Options: []string{"a", "b"}, Default: "a", Placeholder: "p", Prefill: "pre", Timeout: time.Minute}},
		{"answer-value", Answer{ID: "q1", Value: "a"}},
		{"answer-confirm", Answer{ID: "q1", Confirmed: &confirmed}},
		{"answer-cancelled", Answer{ID: "q1", Cancelled: true}},
		{"descriptor", Descriptor{ID: "pi", Name: "Pi", Version: "0.1.0", Protocol: ProtocolVersion}},
		{"probe", ProbeResult{RuntimeVersion: "1.2.3", Capabilities: Capabilities{Questions: true}, Warnings: []string{"w"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			target := reflect.New(reflect.TypeOf(tc.value))
			if reflect.TypeOf(tc.value).Kind() == reflect.Pointer {
				target = reflect.New(reflect.TypeOf(tc.value).Elem())
			}
			if err := json.Unmarshal(raw, target.Interface()); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			want := reflect.ValueOf(tc.value)
			if want.Kind() == reflect.Pointer {
				want = want.Elem()
			}
			if got := target.Elem().Interface(); !reflect.DeepEqual(got, want.Interface()) {
				t.Fatalf("round trip = %#v, want %#v", got, want.Interface())
			}
		})
	}
}

func TestContractEventJSONRoundTrip(t *testing.T) {
	at := time.Date(2026, 11, 8, 14, 52, 5, 0, time.UTC)

	events := map[Kind]Event{
		KindTextDelta:     {Kind: KindTextDelta, Text: "hello"},
		KindTextDone:      {Kind: KindTextDone, Text: "hello world"},
		KindThinkingDelta: {Kind: KindThinkingDelta, Text: "thinking"},
		KindThinkingDone:  {Kind: KindThinkingDone, Text: "thought"},
		KindTurnStarted:   {Kind: KindTurnStarted},
		KindTurnFinished:  {Kind: KindTurnFinished},
		KindToolStarted:   {Kind: KindToolStarted, Tool: &ToolEvent{CallID: "c1", Name: "read"}},
		KindToolUpdated:   {Kind: KindToolUpdated, Tool: &ToolEvent{CallID: "c1", Summary: "half"}},
		KindToolFinished:  {Kind: KindToolFinished, Tool: &ToolEvent{CallID: "c1", IsError: true}},
		KindFileChanged:   {Kind: KindFileChanged, File: &FileChange{Path: "a.go", Tool: "write"}},
		KindUsage:         {Kind: KindUsage, Usage: &Usage{TotalTokens: 9}},
		KindNotice:        {Kind: KindNotice, Notice: &Notice{Level: NoticeInfo, Code: "x", Message: "y"}},
		KindStatus:        {Kind: KindStatus, Text: "running tool"},
		KindInjectionAck:  {Kind: KindInjectionAck, Injection: &InjectionAck{ID: "i1", Disposition: "queued"}},
	}

	if len(events) != len(AllKinds) {
		t.Fatalf("event fixtures = %d, want one per kind (%d)", len(events), len(AllKinds))
	}

	for _, kind := range AllKinds {
		event, ok := events[kind]
		if !ok {
			t.Fatalf("no fixture for kind %q", kind)
		}
		event.Seq = 7
		event.At = at

		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("%s: marshal: %v", kind, err)
		}

		var got Event
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%s: unmarshal: %v", kind, err)
		}
		if !reflect.DeepEqual(got, event) {
			t.Fatalf("%s: round trip = %#v, want %#v", kind, got, event)
		}
	}
}

// TestContractJSONKeys pins every JSON field name. Renaming a field is a
// protocol change, so this test is the tripwire.
func TestContractJSONKeys(t *testing.T) {
	confirmed := true

	cases := []struct {
		name  string
		value any
		want  []string
	}{
		{
			"skill",
			&Skill{Name: "n", Paths: []string{"/p"}},
			[]string{"name", "paths"},
		},
		{
			"model",
			Model{ID: "m", Thinking: "high"},
			[]string{"id", "thinking"},
		},
		{
			"tool-policy",
			ToolPolicy{Allow: []string{"a"}, Deny: []string{"b"}},
			[]string{"allow", "deny"},
		},
		{
			"request",
			Request{
				NodeID:  "n",
				Prompt:  "p",
				Skill:   &Skill{Name: "s"},
				Model:   Model{ID: "m"},
				Tools:   ToolPolicy{Allow: []string{"a"}},
				Workdir: "/w",
				Env:     map[string]string{"K": "V"},
				Trust:   true,
				Config:  json.RawMessage(`{}`),
			},
			[]string{"nodeId", "prompt", "skill", "model", "tools", "workdir", "env", "trust", "config"},
		},
		{
			"usage",
			Usage{InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 4, TotalTokens: 5, CostUSD: 6},
			[]string{"inputTokens", "outputTokens", "cacheReadTokens", "cacheWriteTokens", "totalTokens", "costUsd"},
		},
		{
			"file-change",
			FileChange{Path: "p", Tool: "write"},
			[]string{"path", "tool"},
		},
		{
			"harness-ref",
			HarnessRef{ID: "i", Session: "s", Version: "v"},
			[]string{"id", "session", "version"},
		},
		{
			"failure",
			&Failure{Class: FailureConfig, Message: "m", Detail: "d", Retryable: true},
			[]string{"class", "message", "detail", "retryable"},
		},
		{
			"notice",
			&Notice{Level: NoticeInfo, Code: "c", Message: "m"},
			[]string{"level", "code", "message"},
		},
		{
			"injection-ack",
			&InjectionAck{ID: "i", Disposition: "d", Error: "e"},
			[]string{"id", "disposition", "error"},
		},
		{
			"tool-event",
			ToolEvent{CallID: "c", Name: "n", Args: map[string]any{"a": "b"}, Summary: "s", IsError: true, DurationMS: 1},
			[]string{"callId", "name", "args", "summary", "isError", "durationMs"},
		},
		{
			"result",
			Result{
				Status:  StatusFailed,
				Text:    "t",
				Turns:   1,
				Tools:   2,
				Usage:   Usage{TotalTokens: 3},
				Files:   []FileChange{{Path: "p"}},
				Ref:     HarnessRef{ID: "i"},
				Failure: &Failure{Class: FailureConfig, Message: "m"},
			},
			[]string{"status", "text", "turns", "tools", "usage", "files", "ref", "failure"},
		},
		{
			"event",
			Event{
				Seq:       1,
				At:        time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
				Kind:      KindToolFinished,
				Text:      "t",
				Tool:      &ToolEvent{Name: "n"},
				File:      &FileChange{Path: "p"},
				Usage:     &Usage{TotalTokens: 1},
				Notice:    &Notice{Level: NoticeInfo, Message: "m"},
				Injection: &InjectionAck{ID: "i"},
			},
			[]string{"seq", "at", "kind", "text", "tool", "file", "usage", "notice", "injection"},
		},
		{
			"ask",
			Ask{ID: "i", Kind: AskInput, Title: "t", Message: "m", Options: []string{"a"}, Default: "d", Placeholder: "p", Prefill: "f", Timeout: time.Second},
			[]string{"id", "kind", "title", "message", "options", "default", "placeholder", "prefill", "timeout"},
		},
		{
			"answer",
			Answer{ID: "i", Value: "v", Confirmed: &confirmed, Cancelled: true},
			[]string{"id", "value", "confirmed", "cancelled"},
		},
		{
			"capabilities",
			Capabilities{
				Streaming: true, Thinking: true, ToolEvents: true, FileEvents: true,
				Questions: true, Injection: true, Abort: true, Skills: true,
				ModelSelect: true, ToolPolicy: true, Plugins: true, Usage: true,
				SessionResume: true,
			},
			[]string{
				"streaming", "thinking", "toolEvents", "fileEvents", "questions",
				"injection", "abort", "skills", "modelSelect", "toolPolicy",
				"plugins", "usage", "sessionResume",
			},
		},
		{
			"descriptor",
			Descriptor{ID: "i", Name: "n", Version: "v", Protocol: 1},
			[]string{"id", "name", "version", "protocol"},
		},
		{
			"probe-result",
			ProbeResult{RuntimeVersion: "v", Capabilities: Capabilities{Questions: true}, Warnings: []string{"w"}},
			[]string{"runtimeVersion", "capabilities", "warnings"},
		},
		{
			"injection",
			Injection{ID: "i", Text: "t"},
			[]string{"id", "text"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jsonKeys(t, tc.value)
			want := slices.Clone(tc.want)
			sort.Strings(want)
			if !slices.Equal(got, want) {
				t.Fatalf("JSON keys = %v, want %v", got, want)
			}
		})
	}
}

func jsonKeys(t *testing.T, value any) []string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode object: %v", err)
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
