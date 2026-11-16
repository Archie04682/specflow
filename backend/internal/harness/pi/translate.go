package pi

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"example.com/specflow/backend/internal/harness"
)

// pendingAsk is one specflow_ask call whose dialog has not arrived yet. Pi's
// dialog protocol has no description slot, but the tool's arguments carry the
// question the model asked, and Pi emits the tool start before the dialog.
type pendingAsk struct {
	callID   string
	title    string
	question string
	options  []string
}

// translator turns Pi's session records into contract telemetry and
// accumulates the pieces the terminal result needs.
//
// It is pure translation: it never touches processes or files, so every rule
// below is testable from recorded fixtures.
type translator struct {
	host harness.Host
	now  func() time.Time

	emitMu sync.Mutex

	seq uint64

	turns int
	tools int
	usage harness.Usage
	files []harness.FileChange

	text     strings.Builder
	thinking strings.Builder
	lastText string

	// failureText is the last in-band error Pi has not demonstrably recovered
	// from. Only a later assistant message that completes successfully clears
	// it; a retry announcement alone does not, because the retry may never
	// produce a message.
	failureText string

	running atomic.Bool
	settled bool
	aborted bool

	// toolArgs remembers each call's arguments. Pi sends them on the start
	// record but not on the end record, so the end record alone cannot say
	// which file the call touched.
	toolArgs map[string]record

	// pendingAsks is a short FIFO of specflow_ask calls awaiting their dialog.
	// The entry is dropped when the call's end record arrives, and consumed by
	// the dialog it belongs to.
	pendingAsks []pendingAsk

	unknown map[string]bool
}

func newTranslator(host harness.Host) *translator {
	return &translator{
		host:     host,
		now:      time.Now,
		toolArgs: make(map[string]record),
		unknown:  make(map[string]bool),
	}
}

func (t *translator) emit(kind harness.Kind, mutate func(*harness.Event)) error {
	t.emitMu.Lock()
	defer t.emitMu.Unlock()

	t.seq++
	event := harness.Event{Seq: t.seq, At: t.now(), Kind: kind}
	if mutate != nil {
		mutate(&event)
	}
	return t.host.Emit(event)
}

func (t *translator) emitNotice(level harness.NoticeLevel, code, message string) error {
	return t.emit(harness.KindNotice, func(event *harness.Event) {
		event.Notice = &harness.Notice{Level: level, Code: code, Message: message}
	})
}

// handle consumes one session record.
func (t *translator) handle(rec record) error {
	switch kind := rec.str("type"); kind {
	case "turn_start":
		t.running.Store(true)
		t.turns++
		t.text.Reset()
		t.thinking.Reset()
		return t.emit(harness.KindTurnStarted, nil)

	case "turn_end":
		t.running.Store(false)
		if err := t.emitUsage(); err != nil {
			return err
		}
		return t.emit(harness.KindTurnFinished, nil)

	case "message_update":
		return t.handleMessageUpdate(rec)

	case "message_end":
		t.handleMessageEnd(rec)
		return nil

	case "tool_execution_start", "tool_execution_update", "tool_execution_end":
		return t.handleTool(rec)

	case "agent_end":
		// Deliberately not terminal: Pi may still retry, recover, or run
		// queued work. Only agent_settled ends an execution. A retry
		// announcement is not a recovery: the recorded error stays until a
		// later assistant message completes successfully.
		return t.emitNotice(harness.NoticeInfo, "agent_run_ended", "agent run ended; waiting to settle")

	case "agent_settled":
		t.settled = true
		t.aborted = rec.boolean("aborted")
		return nil

	case "compaction_start":
		return t.emitNotice(harness.NoticeInfo, "compaction", "compacting context")

	case "compaction_end":
		notice := "compaction finished"
		if rec.boolean("aborted") {
			notice = "compaction aborted"
		}
		if err := t.emitNotice(harness.NoticeInfo, "compaction", notice); err != nil {
			return err
		}
		return t.emitUsage()

	case "message_start", "agent_start", "queue_update", "entry_appended",
		"session_info_changed", "thinking_level_changed":
		// Known records with nothing to show the user.
		return nil

	default:
		if strings.Contains(kind, "retry") {
			return t.emitNotice(harness.NoticeWarning, "retry", "the provider stream is being retried")
		}
		return t.handleUnknown(kind)
	}
}

func (t *translator) handleMessageUpdate(rec record) error {
	t.recordUsage(rec)

	update := rec.obj("assistantMessageEvent")
	if update == nil {
		return nil
	}

	switch update.str("type") {
	case "text_delta":
		delta := update.str("delta")
		t.text.WriteString(delta)
		return t.emit(harness.KindTextDelta, func(event *harness.Event) { event.Text = delta })

	case "text_end":
		content := update.str("content")
		t.text.Reset()
		t.text.WriteString(content)
		return t.emit(harness.KindTextDone, func(event *harness.Event) { event.Text = content })

	case "thinking_delta":
		delta := update.str("delta")
		t.thinking.WriteString(delta)
		return t.emit(harness.KindThinkingDelta, func(event *harness.Event) { event.Text = delta })

	case "thinking_end":
		content := update.str("content")
		t.thinking.Reset()
		t.thinking.WriteString(content)
		return t.emit(harness.KindThinkingDone, func(event *harness.Event) { event.Text = content })

	case "error":
		// A provider stream error is normally rewritten into message_end by
		// Pi's agent loop, but the wire variant is admitted, so record it.
		detail := update.str("error")
		if detail == "" {
			detail = update.str("reason")
		}
		if detail == "" {
			detail = "pi reported a provider error"
		}
		t.failureText = detail
		return nil

	default:
		// toolcall_start/toolcall_delta/toolcall_end describe what the model
		// intends to call. The execution that follows is the real signal, so
		// these are not duplicated as events.
		return nil
	}
}

func (t *translator) handleMessageEnd(rec record) {
	message := rec.obj("message")
	if message == nil || message.str("role") != "assistant" {
		return
	}
	if text := extractText(message); text != "" {
		t.lastText = text
	}

	// The completed assistant message is authoritative: an error stop reason
	// is a terminal provider failure, and a normal one clears an error that a
	// retry already recovered from.
	switch message.str("stopReason") {
	case "error":
		detail := message.str("errorMessage")
		if detail == "" {
			detail = "pi reported a provider error"
		}
		t.failureText = detail
	case "stop", "toolUse", "length", "aborted":
		t.clearFailure()
	}
}

// executionError reports the last in-band error Pi did not recover from.
func (t *translator) executionError() string { return t.failureText }

func (t *translator) clearFailure() { t.failureText = "" }

func (t *translator) handleTool(rec record) error {
	callID := rec.str("toolCallId")
	name := rec.str("toolName")

	args := rec.obj("args")
	if args != nil {
		t.toolArgs[callID] = args
	}

	switch rec.str("type") {
	case "tool_execution_start":
		t.recordAsk(callID, name, args)
		t.tools++
		return t.emit(harness.KindToolStarted, func(event *harness.Event) {
			event.Tool = &harness.ToolEvent{CallID: callID, Name: name, Args: rec.obj("args")}
		})

	case "tool_execution_update":
		summary := summarize(rec.obj("partialResult"))
		return t.emit(harness.KindToolUpdated, func(event *harness.Event) {
			event.Tool = &harness.ToolEvent{CallID: callID, Name: name, Summary: summary}
		})

	case "tool_execution_end":
		t.dropAsk(callID)
		summary := summarize(rec.obj("result"))
		isError := rec.boolean("isError")
		duration := rec.integer("durationMs")
		if err := t.emit(harness.KindToolFinished, func(event *harness.Event) {
			event.Tool = &harness.ToolEvent{
				CallID:     callID,
				Name:       name,
				Summary:    summary,
				IsError:    isError,
				DurationMS: duration,
			}
		}); err != nil {
			return err
		}

		// The end record omits args; fall back to the ones the start record
		// carried so the change can still be attributed.
		if args == nil {
			args = t.toolArgs[callID]
		}
		delete(t.toolArgs, callID)

		path := touchedPath(name, args)
		if path == "" {
			return nil
		}
		change := harness.FileChange{Path: path, Tool: name}
		t.files = append(t.files, change)
		return t.emit(harness.KindFileChanged, func(event *harness.Event) {
			event.File = &change
		})
	}

	return nil
}

func (t *translator) recordUsage(rec record) {
	usage := rec.obj("usage")
	if usage == nil {
		return
	}

	t.usage = harness.Usage{
		InputTokens:      usage.integer("input"),
		OutputTokens:     usage.integer("output"),
		CacheReadTokens:  usage.integer("cacheRead"),
		CacheWriteTokens: usage.integer("cacheWrite"),
		TotalTokens:      usage.integer("totalTokens"),
	}
	if cost := usage.obj("cost"); cost != nil {
		t.usage.CostUSD = cost.number("total")
	}
}

func (t *translator) emitUsage() error {
	if t.usage == (harness.Usage{}) {
		return nil
	}
	usage := t.usage
	return t.emit(harness.KindUsage, func(event *harness.Event) { event.Usage = &usage })
}

func (t *translator) handleUnknown(kind string) error {
	if t.unknown[kind] {
		return nil
	}
	t.unknown[kind] = true
	return t.emitNotice(harness.NoticeWarning, "unknown_record", fmt.Sprintf("ignoring unsupported pi record %q", kind))
}

// recordAsk remembers a specflow_ask call's arguments so the dialog request
// that follows can report the question the model actually asked. Pi's dialog
// protocol drops that text for select, input, and editor dialogs.
func (t *translator) recordAsk(callID, toolName string, args record) {
	if t == nil || toolName != askToolName || args == nil {
		return
	}
	question := args.str("question")
	if question == "" {
		return
	}
	t.pendingAsks = append(t.pendingAsks, pendingAsk{
		callID:   callID,
		title:    args.str("title"),
		question: question,
		options:  stringList(args.list("options")),
	})
}

// dropAsk discards a call's pending entry when its tool end arrives. A dialog
// that already consumed the entry leaves nothing to drop.
func (t *translator) dropAsk(callID string) {
	if t == nil || callID == "" {
		return
	}
	for index, pending := range t.pendingAsks {
		if pending.callID == callID {
			t.pendingAsks = append(t.pendingAsks[:index], t.pendingAsks[index+1:]...)
			return
		}
	}
}

// mergeAsk fills a dialog ask from the question tool call it belongs to. The
// oldest entry whose title matches wins, else the oldest entry. A confirm
// dialog already carries its own message and keeps it. An unmatched dialog is
// returned unchanged and remains answerable.
func (t *translator) mergeAsk(ask harness.Ask) harness.Ask {
	pending, ok := t.takeAsk(ask.Title)
	if !ok {
		return ask
	}
	if ask.Message == "" {
		ask.Message = pending.question
	}
	if ask.Title == "" {
		ask.Title = pending.title
	}
	if len(ask.Options) == 0 && len(pending.options) > 0 {
		ask.Options = append([]string(nil), pending.options...)
	}
	return ask
}

// takeAsk removes and returns the pending entry a dialog consumes. Title
// preference keeps two outstanding asks from crossing; the oldest entry is the
// fallback when titles do not match.
func (t *translator) takeAsk(dialogTitle string) (pendingAsk, bool) {
	if t == nil || len(t.pendingAsks) == 0 {
		return pendingAsk{}, false
	}

	index := 0
	if dialogTitle != "" {
		for candidate, pending := range t.pendingAsks {
			if pending.title == dialogTitle {
				index = candidate
				break
			}
		}
	}
	pending := t.pendingAsks[index]
	t.pendingAsks = append(t.pendingAsks[:index], t.pendingAsks[index+1:]...)
	return pending, true
}

// handleUI translates an extension UI request. Dialogs become asks; the
// fire-and-forget methods become telemetry. It reports whether the record was
// a dialog so the caller knows to answer it.
func (t *translator) handleUI(rec record) (harness.Ask, bool, error) {
	if rec.str("type") != "extension_ui_request" {
		return harness.Ask{}, false, nil
	}

	title := rec.str("title")
	timeout := time.Duration(rec.integer("timeout")) * time.Millisecond

	// The bridge announces itself once per session so Probe can confirm it
	// loaded. It is a handshake, not something the operator needs to read.
	if rec.str("method") == "notify" && rec.str("message") == BridgeMarker {
		return harness.Ask{}, false, nil
	}

	switch method := rec.str("method"); method {
	case "select":
		return t.mergeAsk(harness.Ask{
			ID:      rec.str("id"),
			Kind:    harness.AskSelect,
			Title:   title,
			Message: rec.str("message"),
			Options: stringList(rec.list("options")),
			Timeout: timeout,
		}), true, nil

	case "confirm":
		return t.mergeAsk(harness.Ask{
			ID:      rec.str("id"),
			Kind:    harness.AskConfirm,
			Title:   title,
			Message: rec.str("message"),
			Timeout: timeout,
		}), true, nil

	case "input":
		return t.mergeAsk(harness.Ask{
			ID:          rec.str("id"),
			Kind:        harness.AskInput,
			Title:       title,
			Placeholder: rec.str("placeholder"),
			Timeout:     timeout,
		}), true, nil

	case "editor":
		return t.mergeAsk(harness.Ask{
			ID:      rec.str("id"),
			Kind:    harness.AskEditor,
			Title:   title,
			Prefill: rec.str("prefill"),
			Timeout: timeout,
		}), true, nil

	case "notify":
		return harness.Ask{}, false, t.emitNotice(notifyLevel(rec.str("notifyType")), "pi_notify", rec.str("message"))

	case "setStatus":
		return harness.Ask{}, false, t.emit(harness.KindStatus, func(event *harness.Event) {
			event.Text = rec.str("statusText")
		})

	default:
		// setWidget, setTitle, set_editor_text and anything newer are
		// presentation-only; they are not ours to render.
		return harness.Ask{}, false, nil
	}
}

// isRunning reports whether a turn is in flight, which decides whether an
// injected message is steered or queued as a follow-up.
func (t *translator) isRunning() bool { return t.running.Load() }

// result assembles the terminal outcome from accumulated state.
func (t *translator) result(status harness.Status) harness.Result {
	text := t.lastText
	if text == "" {
		text = t.text.String()
	}

	return harness.Result{
		Status: status,
		Text:   text,
		Turns:  t.turns,
		Tools:  t.tools,
		Usage:  t.usage,
		Files:  append([]harness.FileChange(nil), t.files...),
	}
}

// extractText concatenates the text blocks of a message or tool result.
func extractText(value record) string {
	if value == nil {
		return ""
	}

	switch content := value["content"].(type) {
	case string:
		return content
	case []any:
		var out strings.Builder
		for _, item := range content {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := block["text"].(string); ok {
				out.WriteString(text)
			}
		}
		return out.String()
	default:
		return ""
	}
}

// toolSummaryLimit is how much tool output telemetry retains. It matches the
// frontend stream's MAX_SUMMARY, so expanding a tool card shows the whole
// retained output instead of a 2,000-character cut.
const toolSummaryLimit = 16_000

// summarize renders tool output for telemetry. It is bounded: the full output
// is the harness's business, and telemetry is not an artifact.
func summarize(result record) string {
	text := strings.TrimSpace(extractText(result))
	if len(text) <= toolSummaryLimit {
		return text
	}
	return truncateRunes(text, toolSummaryLimit) + "..."
}

// truncateRunes cuts text at limit bytes without splitting a multi-byte
// character, so a summary is always valid UTF-8.
func truncateRunes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// touchedPath reports the file a write or edit tool touched. Other tools may
// change files too (bash can), but only the explicit ones can be attributed.
func touchedPath(toolName string, args record) string {
	if toolName != "write" && toolName != "edit" {
		return ""
	}
	if args == nil {
		return ""
	}
	for _, key := range []string{"path", "file_path", "filePath"} {
		if path := args.str(key); path != "" {
			return path
		}
	}
	return ""
}

func stringList(values []any) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func notifyLevel(value string) harness.NoticeLevel {
	switch value {
	case "warning":
		return harness.NoticeWarning
	case "error":
		return harness.NoticeError
	default:
		return harness.NoticeInfo
	}
}
