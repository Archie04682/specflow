package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ProtocolVersion is the version of the adapter contract. It changes only when
// a message shape changes incompatibly, because adapters may be built against
// an older contract.
const ProtocolVersion = 1

// Descriptor identifies an adapter and the contract it implements.
type Descriptor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
}

// Capabilities is what an adapter can actually do in the current environment.
//
// A flag is true only when the feature works. Declaration does not require
// executing work: Capabilities is available before Probe, and Probe may report
// a narrower effective set.
type Capabilities struct {
	Streaming     bool `json:"streaming"`
	Thinking      bool `json:"thinking"`
	ToolEvents    bool `json:"toolEvents"`
	FileEvents    bool `json:"fileEvents"`
	Questions     bool `json:"questions"`
	Injection     bool `json:"injection"`
	Abort         bool `json:"abort"`
	Skills        bool `json:"skills"`
	ModelSelect   bool `json:"modelSelect"`
	ToolPolicy    bool `json:"toolPolicy"`
	Plugins       bool `json:"plugins"`
	Usage         bool `json:"usage"`
	SessionResume bool `json:"sessionResume"`
}

// ProbeResult is the outcome of inspecting the runtime without doing work.
//
// Capabilities may be narrower than Adapter.Capabilities: a component that
// failed to initialize (for example an optional bridge) is reported as false
// here so preflight can refuse nodes that need it.
type ProbeResult struct {
	RuntimeVersion string       `json:"runtimeVersion,omitempty"`
	Capabilities   Capabilities `json:"capabilities"`
	Warnings       []string     `json:"warnings,omitempty"`
}

// Adapter controls one coding-agent runtime.
//
// Run executes exactly one node execution. The returned error is non-nil
// exactly when Result.Status is not StatusCompleted, and it wraps a *Failure.
type Adapter interface {
	// Descriptor identifies the adapter. It must not start a process.
	Descriptor() Descriptor
	// Capabilities declares what the adapter supports before any probe. It
	// must not start a process or execute work.
	Capabilities() Capabilities
	// Probe inspects the runtime: executable presence, version compatibility,
	// and whether optional components load. It takes no node work.
	Probe(ctx context.Context) (ProbeResult, error)
	// Run executes one node execution: start the runtime, submit the request,
	// stream telemetry, answer questions, accept injections, and return one
	// terminal result.
	Run(ctx context.Context, req Request, host Host) (Result, error)
}

// Host is the orchestrator's side of an execution.
//
// Emit may block. That is deliberate: it propagates backpressure to the
// harness, which is expected to honor it. Implementations must consume
// promptly and must never drop or reorder events.
type Host interface {
	Emit(Event) error
	Ask(ctx context.Context, ask Ask) (Answer, error)
	Injections() <-chan Injection
}

// Skill is a named instruction bundle the orchestrator resolved for a node.
//
// Paths are absolute directories the harness can load; the adapter decides
// how to use them. A harness that resolves skills by name itself may ignore
// Paths.
type Skill struct {
	Name  string   `json:"name"`
	Paths []string `json:"paths,omitempty"`
}

// SkillList returns the request's skill bindings. Skills is the current form;
// Skill remains for adapters constructed before the plural field existed, and
// for old tests and plans that still carry one binding.
func (r Request) SkillList() []Skill {
	if len(r.Skills) > 0 {
		return r.Skills
	}
	if r.Skill != nil && r.Skill.Name != "" {
		return []Skill{*r.Skill}
	}
	return nil
}

// Model selects the model and reasoning effort for one execution.
type Model struct {
	ID       string `json:"id,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

// ToolPolicy narrows the tools a harness may use.
//
// An empty Allow keeps the harness default selection. A non-empty Allow is an
// allowlist that replaces the default selection. Deny always removes tools.
type ToolPolicy struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// Request is everything an adapter needs to execute one node.
//
// The orchestrator renders Prompt and resolves Skill, so the adapter never
// needs to know about workflows, graphs, or prior nodes. Config is opaque to
// the orchestrator: the adapter parses and validates it.
type Request struct {
	NodeID  string            `json:"nodeId"`
	Prompt  string            `json:"prompt"`
	Skill   *Skill            `json:"skill,omitempty"`
	Skills  []Skill           `json:"skills,omitempty"`
	Model   Model             `json:"model,omitempty"`
	Tools   ToolPolicy        `json:"tools,omitempty"`
	Workdir string            `json:"workdir"`
	Env     map[string]string `json:"env,omitempty"`
	Trust   bool              `json:"trust,omitempty"`
	Config  json.RawMessage   `json:"config,omitempty"`
}

// Status is the single terminal outcome of an execution.
type Status string

const (
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusAborted   Status = "aborted"
	StatusTimedOut  Status = "timed_out"
	StatusCancelled Status = "cancelled"
)

// AllStatuses lists every terminal status, for tests and exhaustive switches.
var AllStatuses = []Status{StatusCompleted, StatusFailed, StatusAborted, StatusTimedOut, StatusCancelled}

// Usage is the token and cost accounting reported by the harness, when it
// reports one at all.
type Usage struct {
	InputTokens      int64   `json:"inputTokens,omitempty"`
	OutputTokens     int64   `json:"outputTokens,omitempty"`
	CacheReadTokens  int64   `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64   `json:"cacheWriteTokens,omitempty"`
	TotalTokens      int64   `json:"totalTokens,omitempty"`
	CostUSD          float64 `json:"costUsd,omitempty"`
}

// FileChange is a best-effort record of a file the harness reported touching.
// It is not a diff and it is not authoritative: artifacts come from the
// repository, not from telemetry.
type FileChange struct {
	Path string `json:"path"`
	Tool string `json:"tool,omitempty"`
}

// HarnessRef is the runtime identity behind an execution, for diagnostics.
type HarnessRef struct {
	ID      string `json:"id,omitempty"`
	Session string `json:"session,omitempty"`
	Version string `json:"version,omitempty"`
}

// Result is the one terminal outcome of an execution.
type Result struct {
	Status  Status       `json:"status"`
	Text    string       `json:"text,omitempty"`
	Turns   int          `json:"turns,omitempty"`
	Tools   int          `json:"tools,omitempty"`
	Usage   Usage        `json:"usage,omitempty"`
	Files   []FileChange `json:"files,omitempty"`
	Ref     HarnessRef   `json:"ref,omitempty"`
	Failure *Failure     `json:"failure,omitempty"`
}

// Err reports the execution as an error when it did not complete.
//
// A non-completed result without a recorded Failure still yields a typed
// error: an adapter that returns one violates the contract, and callers must
// not have to handle an "errorless failure".
func (r Result) Err() error {
	if r.Status == StatusCompleted {
		return nil
	}
	if r.Failure != nil {
		return r.Failure
	}
	return &Failure{
		Class:   FailureProtocol,
		Message: fmt.Sprintf("execution ended with status %q without a failure", r.Status),
	}
}

// Kind classifies a telemetry event. Telemetry is optional: an adapter that
// emits none still completes, and consumers must not require any of them.
type Kind string

const (
	KindTextDelta     Kind = "text_delta"
	KindTextDone      Kind = "text_done"
	KindThinkingDelta Kind = "thinking_delta"
	KindThinkingDone  Kind = "thinking_done"
	KindTurnStarted   Kind = "turn_started"
	KindTurnFinished  Kind = "turn_finished"
	KindToolStarted   Kind = "tool_started"
	KindToolUpdated   Kind = "tool_updated"
	KindToolFinished  Kind = "tool_finished"
	KindFileChanged   Kind = "file_changed"
	KindUsage         Kind = "usage"
	KindNotice        Kind = "notice"
	KindStatus        Kind = "status"
	KindInjectionAck  Kind = "injection_ack"
)

// AllKinds lists every event kind, for tests and exhaustive switches.
var AllKinds = []Kind{
	KindTextDelta, KindTextDone, KindThinkingDelta, KindThinkingDone,
	KindTurnStarted, KindTurnFinished, KindToolStarted, KindToolUpdated,
	KindToolFinished, KindFileChanged, KindUsage, KindNotice, KindStatus,
	KindInjectionAck,
}

// ToolEvent carries tool lifecycle telemetry correlated by CallID.
type ToolEvent struct {
	CallID     string         `json:"callId,omitempty"`
	Name       string         `json:"name,omitempty"`
	Args       map[string]any `json:"args,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	IsError    bool           `json:"isError,omitempty"`
	DurationMS int64          `json:"durationMs,omitempty"`
}

// NoticeLevel classifies a notice.
type NoticeLevel string

const (
	NoticeInfo    NoticeLevel = "info"
	NoticeWarning NoticeLevel = "warning"
	NoticeError   NoticeLevel = "error"
)

// Notice reports something the user should know that is not a terminal
// outcome: compaction, retries, and unrecognized records.
type Notice struct {
	Level   NoticeLevel `json:"level"`
	Code    string      `json:"code,omitempty"`
	Message string      `json:"message"`
}

// InjectionAck reports the disposition of an injected message.
type InjectionAck struct {
	ID          string `json:"id"`
	Disposition string `json:"disposition,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Event is one telemetry record. Exactly one payload field is populated
// according to Kind; consumers must tolerate an absent payload.
type Event struct {
	Seq       uint64        `json:"seq"`
	At        time.Time     `json:"at"`
	Kind      Kind          `json:"kind"`
	Text      string        `json:"text,omitempty"`
	Tool      *ToolEvent    `json:"tool,omitempty"`
	File      *FileChange   `json:"file,omitempty"`
	Usage     *Usage        `json:"usage,omitempty"`
	Notice    *Notice       `json:"notice,omitempty"`
	Injection *InjectionAck `json:"injection,omitempty"`
}

// AskKind is the shape of the answer a question expects.
type AskKind string

const (
	AskInput   AskKind = "input"
	AskSelect  AskKind = "select"
	AskConfirm AskKind = "confirm"
	AskEditor  AskKind = "editor"
)

// Ask is a question the harness needs answered before it can continue.
//
// Timeout is a hint: the host decides the deadline and must answer with
// Cancelled when it expires.
type Ask struct {
	ID          string        `json:"id"`
	Kind        AskKind       `json:"kind"`
	Title       string        `json:"title,omitempty"`
	Message     string        `json:"message,omitempty"`
	Options     []string      `json:"options,omitempty"`
	Default     string        `json:"default,omitempty"`
	Placeholder string        `json:"placeholder,omitempty"`
	Prefill     string        `json:"prefill,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
}

// Answer resolves an Ask. Value carries text, select, and editor answers;
// Confirmed carries a confirmation; Cancelled means no answer is available.
type Answer struct {
	ID        string `json:"id"`
	Value     string `json:"value,omitempty"`
	Confirmed *bool  `json:"confirmed,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// Injection is a user message delivered into a running execution.
type Injection struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// FailureClass tells the orchestrator what kind of failure occurred, which
// decides whether retrying could plausibly succeed.
type FailureClass string

const (
	FailureAdapterMissing FailureClass = "adapter_missing"
	FailureUnsupported    FailureClass = "unsupported"
	FailureConfig         FailureClass = "config"
	FailureAuth           FailureClass = "auth"
	FailureProvider       FailureClass = "provider"
	FailureStart          FailureClass = "start"
	FailureProtocol       FailureClass = "protocol"
	FailureHarnessExit    FailureClass = "harness_exit"
	FailureTimeout        FailureClass = "timeout"
	FailureCancelled      FailureClass = "cancelled"
)

// AllFailureClasses lists every class, for tests and exhaustive switches.
var AllFailureClasses = []FailureClass{
	FailureAdapterMissing, FailureUnsupported, FailureConfig, FailureAuth,
	FailureProvider, FailureStart, FailureProtocol, FailureHarnessExit,
	FailureTimeout, FailureCancelled,
}

// Failure is a classified execution failure.
type Failure struct {
	Class     FailureClass `json:"class"`
	Message   string       `json:"message"`
	Detail    string       `json:"detail,omitempty"`
	Retryable bool         `json:"retryable"`
}

// Error implements error.
func (f *Failure) Error() string {
	if f == nil {
		return "<nil harness failure>"
	}
	if f.Detail == "" {
		return fmt.Sprintf("%s: %s", f.Class, f.Message)
	}
	return fmt.Sprintf("%s: %s: %s", f.Class, f.Message, f.Detail)
}
