package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"example.com/specflow/backend/internal/harness"
)

const (
	// DescriptorID is the harness identifier used by workflows.
	DescriptorID = "pi"

	defaultBinary        = "pi"
	defaultMinVersion    = "1.0.0"
	defaultStartup       = 15 * time.Second
	defaultCommand       = 30 * time.Second
	defaultGrace         = 3 * time.Second
	defaultBridgeConfirm = 500 * time.Millisecond

	// askToolName is the tool the embedded bridge extension registers.
	askToolName = "specflow_ask"
)

// Options configures the adapter. Zero values fall back to defaults, which is
// what the server uses.
type Options struct {
	// Binary is the executable name or path. Empty means "pi" from PATH.
	Binary string
	// MinVersion is the oldest Pi release the adapter believes it supports.
	MinVersion string
	// CacheDir is where the bridge extension is materialized. Empty means the
	// platform cache directory.
	CacheDir string
	// StartupTimeout bounds process start plus the readiness handshake.
	StartupTimeout time.Duration
	// CommandTimeout bounds a single command's response.
	CommandTimeout time.Duration
	// ShutdownGrace bounds each shutdown escalation step.
	ShutdownGrace time.Duration
	// BridgeConfirm bounds the wait for the bridge-ready marker during Probe.
	BridgeConfirm time.Duration
}

func (o Options) withDefaults() Options {
	if o.Binary == "" {
		o.Binary = defaultBinary
	}
	if o.MinVersion == "" {
		o.MinVersion = defaultMinVersion
	}
	if o.StartupTimeout <= 0 {
		o.StartupTimeout = defaultStartup
	}
	if o.CommandTimeout <= 0 {
		o.CommandTimeout = defaultCommand
	}
	if o.ShutdownGrace <= 0 {
		o.ShutdownGrace = defaultGrace
	}
	if o.BridgeConfirm <= 0 {
		o.BridgeConfirm = defaultBridgeConfirm
	}
	return o
}

// Config is the adapter-specific part of a request, decoded from
// harness.Request.Config. The orchestrator never interprets it.
type Config struct {
	Provider string `json:"provider,omitempty"`
	Offline  bool   `json:"offline,omitempty"`
}

// Adapter drives Pi in RPC mode.
type Adapter struct {
	opts Options
}

// New returns a Pi adapter.
func New(opts Options) *Adapter {
	return &Adapter{opts: opts.withDefaults()}
}

// Descriptor identifies the adapter.
func (a *Adapter) Descriptor() harness.Descriptor {
	return harness.Descriptor{
		ID:       DescriptorID,
		Name:     "Pi",
		Version:  "0.1.0",
		Protocol: harness.ProtocolVersion,
	}
}

// Capabilities is the declared feature set. Probe may report fewer.
func (a *Adapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{
		Streaming:     true,
		Thinking:      true,
		ToolEvents:    true,
		FileEvents:    true,
		Questions:     true,
		Injection:     true,
		Abort:         true,
		Skills:        true,
		ModelSelect:   true,
		ToolPolicy:    true,
		Plugins:       true,
		Usage:         true,
		SessionResume: false,
	}
}

// Probe checks the executable, the version, and whether the bridge loads.
func (a *Adapter) Probe(ctx context.Context) (harness.ProbeResult, error) {
	result := harness.ProbeResult{Capabilities: a.Capabilities()}

	binary, err := exec.LookPath(a.opts.Binary)
	if err != nil {
		return result, &harness.Failure{
			Class:   harness.FailureAdapterMissing,
			Message: fmt.Sprintf("%s executable not found", a.opts.Binary),
			Detail:  "install Pi or set the adapter's binary path",
		}
	}

	version := a.runtimeVersion(ctx, binary)
	result.RuntimeVersion = version

	if version != "" && compareVersions(version, a.opts.MinVersion) < 0 {
		return result, &harness.Failure{
			Class:   harness.FailureConfig,
			Message: fmt.Sprintf("pi %s is older than the minimum supported version %s", version, a.opts.MinVersion),
		}
	}

	bridge, err := materializeBridge(a.opts.CacheDir)
	if err != nil {
		result.Capabilities.Questions = false
		result.Warnings = append(result.Warnings, "bridge extension unavailable: "+err.Error())
		return result, nil
	}

	confirmed, warning := a.probeBridge(ctx, binary, bridge)
	if warning != "" {
		result.Capabilities.Questions = false
		result.Warnings = append(result.Warnings, warning)
	} else if !confirmed {
		result.Warnings = append(result.Warnings, "bridge extension did not confirm it loaded; questions may fail")
	}

	return result, nil
}

func (a *Adapter) runtimeVersion(ctx context.Context, binary string) string {
	ctx, cancel := context.WithTimeout(ctx, a.opts.CommandTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	if err != nil {
		return ""
	}

	match := semverPattern.FindStringSubmatch(string(output))
	if match == nil {
		return ""
	}
	return strings.Join(match[1:], ".")
}

// probeBridge starts a short session with the extension loaded. A session that
// fails to start or cannot answer is the observable form of "the bridge does
// not load".
func (a *Adapter) probeBridge(ctx context.Context, binary, bridge string) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, a.opts.StartupTimeout)
	defer cancel()

	proc, err := startProcess(binary, []string{"--mode", "rpc", "--no-session", "-e", bridge}, "", os.Environ())
	if err != nil {
		return false, "bridge probe could not start pi: " + err.Error()
	}
	defer proc.stop(a.opts.ShutdownGrace)

	c := proc.client()
	c.start()

	if _, err := c.call(ctx, record{"type": "get_state"}); err != nil {
		detail := strings.TrimSpace(proc.stderr.String())
		if detail != "" {
			return false, "bridge extension failed to load: " + detail
		}
		return false, "bridge extension failed to load: " + err.Error()
	}

	confirmCtx, cancelConfirm := context.WithTimeout(ctx, a.opts.BridgeConfirm)
	defer cancelConfirm()

	for {
		rec, ok := c.events.next(confirmCtx)
		if !ok {
			return false, ""
		}
		if rec.str("type") == "extension_ui_request" && rec.str("message") == BridgeMarker {
			return true, ""
		}
	}
}

// Run executes one node execution on a fresh Pi process.
func (a *Adapter) Run(ctx context.Context, req harness.Request, host harness.Host) (harness.Result, error) {
	cfg, err := parseConfig(req.Config)
	if err != nil {
		return failureResult(harness.FailureConfig, "invalid pi node configuration", err.Error())
	}

	binary, err := exec.LookPath(a.opts.Binary)
	if err != nil {
		return failureResult(harness.FailureAdapterMissing, fmt.Sprintf("%s executable not found", a.opts.Binary), "install Pi or set the adapter's binary path")
	}

	bridge, err := materializeBridge(a.opts.CacheDir)
	if err != nil {
		return failureResult(harness.FailureConfig, "bridge extension unavailable", err.Error())
	}

	prompt, err := promptForRequest(req)
	if err != nil {
		return failureResult(harness.FailureConfig, "bound skill could not be read", err.Error())
	}

	proc, err := startProcess(binary, buildArgs(req, cfg, bridge), req.Workdir, environ(req.Env))
	if err != nil {
		return failureResult(harness.FailureStart, "could not start pi", err.Error())
	}
	defer proc.stop(a.opts.ShutdownGrace)

	c := proc.client()
	c.start()

	tr := newTranslator(host)

	handshakeCtx, cancelHandshake := context.WithTimeout(ctx, a.opts.StartupTimeout)
	state, err := c.call(handshakeCtx, record{"type": "get_state"})
	cancelHandshake()
	if err != nil {
		if ctx.Err() != nil {
			return cancelledResult(ctx, a, c, tr)
		}
		return startFailure(err, proc)
	}

	sessionID := state.obj("data").str("sessionId")

	for _, skill := range req.SkillList() {
		if skill.Name == "" {
			continue
		}
		if err := a.verifySkill(ctx, c, skill.Name, tr); err != nil {
			return failureResult(harness.FailureConfig, "bound skill did not load", err.Error())
		}
	}

	injectionCtx, stopInjections := context.WithCancel(ctx)
	defer stopInjections()
	go a.injectionPump(injectionCtx, c, host, tr)

	promptCtx, cancelPrompt := context.WithTimeout(ctx, a.opts.CommandTimeout)
	reply, err := c.call(promptCtx, record{"type": "prompt", "message": prompt})
	cancelPrompt()
	if err != nil {
		if ctx.Err() != nil {
			return cancelledResult(ctx, a, c, tr)
		}
		return failureResult(harness.FailureProtocol, "pi did not accept the prompt", failureDetail(err, proc))
	}

	if success, _ := reply["success"].(bool); !success {
		detail := reply.str("error")
		class, _ := classifyErrorText(detail)
		if class == harness.FailureProvider {
			class = harness.FailureConfig
		}
		return failureResult(class, "pi rejected the prompt", detail)
	}

	disposition := reply.obj("data").str("disposition")
	switch disposition {
	case "started", "queued":
	case "handled":
		return failureResult(
			harness.FailureProtocol,
			"pi handled the prompt without running it",
			`disposition was "handled": an extension command may share the skill name; rename the skill or the command`,
		)
	default:
		return failureResult(harness.FailureProtocol, "pi returned an unknown prompt disposition", disposition)
	}

	for {
		rec, ok := c.events.next(ctx)
		if !ok {
			if ctx.Err() != nil {
				return cancelledResult(ctx, a, c, tr)
			}
			return a.streamEnded(c, tr, proc)
		}

		if rec.str("type") == "extension_ui_request" {
			ask, isDialog, err := tr.handleUI(rec)
			if err != nil {
				return failureResult(harness.FailureProtocol, "could not record an extension request", err.Error())
			}
			if isDialog {
				go a.answerAsk(ctx, c, host, ask)
			}
			continue
		}

		if err := tr.handle(rec); err != nil {
			return failureResult(harness.FailureProtocol, "could not record telemetry", err.Error())
		}
		if tr.settled {
			break
		}
	}

	return settledResult(tr, sessionID)
}

// settledResult assembles the terminal outcome after Pi settles. An error Pi
// reported in-band is a failure even though the process ended normally;
// treating the settle marker alone as success would report a failed model call
// as completed work.
func settledResult(tr *translator, sessionID string) (harness.Result, error) {
	status := harness.StatusCompleted
	if tr.aborted {
		status = harness.StatusAborted
	}

	result := tr.result(status)
	result.Ref = harness.HarnessRef{ID: DescriptorID, Session: sessionID}
	if status != harness.StatusCompleted {
		result.Failure = &harness.Failure{
			Class:   harness.FailureCancelled,
			Message: "pi aborted the execution",
		}
		return result, result.Err()
	}

	if detail := tr.executionError(); detail != "" {
		result.Status = harness.StatusFailed
		result.Failure = executionFailure(detail)
		return result, result.Err()
	}
	return result, nil
}

// executionFailure classifies an in-band execution error. Pi reports provider
// failures as text, so the table is deliberately small and the full text is
// preserved as detail.
func executionFailure(detail string) *harness.Failure {
	class, retryable := classifyErrorText(detail)
	return &harness.Failure{
		Class:     class,
		Message:   "pi reported an execution error",
		Detail:    detail,
		Retryable: retryable,
	}
}

// classifyErrorText maps an error message to a failure class. Authentication
// and permanent request errors are not retryable; everything else is treated
// as a transient provider error.
func classifyErrorText(detail string) (harness.FailureClass, bool) {
	lower := strings.ToLower(detail)
	switch {
	case containsAny(lower,
		"no api key", "missing api key", "invalid api key", "api key not found",
		"authentication failed", "authentication required", "unauthorized",
		"invalid key", "forbidden", "status 401", "status 403", "http 401", "http 403"):
		return harness.FailureAuth, false
	case containsAny(lower, "invalid model", "unknown model", "model not found", "invalid request", "bad request", "status 400", "status 404", "http 400", "http 404"):
		return harness.FailureConfig, false
	case strings.Contains(lower, "model") && strings.Contains(lower, "not found"):
		return harness.FailureConfig, false
	default:
		return harness.FailureProvider, true
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// streamEnded reports an execution whose stream closed before settling.
func (a *Adapter) streamEnded(c *client, tr *translator, proc *process) (harness.Result, error) {
	if tr.settled {
		return settledResult(tr, "")
	}

	if failure := c.protocolFailure(); failure != nil {
		if detail := strings.TrimSpace(proc.stderr.String()); detail != "" {
			failure.Detail = strings.TrimSpace(failure.Detail + "\n" + detail)
		}
		result := tr.result(harness.StatusFailed)
		result.Failure = failure
		return result, result.Err()
	}

	detail := failureDetail(c.readFailure(), proc)
	if err := proc.exitErr(); err != nil {
		detail = strings.TrimSpace(detail + "\n" + err.Error())
	}

	failure := &harness.Failure{
		Class:     harness.FailureHarnessExit,
		Message:   "pi exited before settling",
		Detail:    detail,
		Retryable: true,
	}
	result := tr.result(harness.StatusFailed)
	result.Failure = failure
	return result, result.Err()
}

// cancelledResult stops the execution and reports why it ended. A deadline is
// a timeout, not a user cancellation: the orchestrator retries a timeout but
// must not silently turn a cancelled run into a retryable one.
func cancelledResult(ctx context.Context, a *Adapter, c *client, tr *translator) (harness.Result, error) {
	abortCtx, cancel := context.WithTimeout(context.Background(), a.opts.ShutdownGrace)
	_, _ = c.call(abortCtx, record{"type": "abort"})
	cancel()

	drainUntilSettled(c, tr, a.opts.ShutdownGrace)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result := tr.result(harness.StatusTimedOut)
		result.Failure = &harness.Failure{
			Class:     harness.FailureTimeout,
			Message:   "execution timed out",
			Detail:    ctx.Err().Error(),
			Retryable: true,
		}
		return result, result.Err()
	}

	result := tr.result(harness.StatusCancelled)
	result.Failure = &harness.Failure{
		Class:   harness.FailureCancelled,
		Message: "execution cancelled",
	}
	if ctx.Err() != nil {
		result.Failure.Message = "execution cancelled: " + ctx.Err().Error()
	}
	return result, result.Err()
}

func drainUntilSettled(c *client, tr *translator, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for !tr.settled {
		rec, ok := c.events.next(ctx)
		if !ok {
			return
		}
		if rec.str("type") == "extension_ui_request" {
			_, _, _ = tr.handleUI(rec)
			continue
		}
		if err := tr.handle(rec); err != nil {
			return
		}
	}
}

// verifySkill confirms that Pi actually registered the skill the node bound.
// A directory can exist, pass compilation, and still be ignored by Pi — for
// example a SKILL.md without a description — so the filesystem is not
// authoritative. An older Pi without get_commands is reported and allowed to
// continue rather than failing a node that may still work.
func (a *Adapter) verifySkill(ctx context.Context, c *client, name string, tr *translator) error {
	callCtx, cancel := context.WithTimeout(ctx, a.opts.CommandTimeout)
	response, err := c.call(callCtx, record{"type": "get_commands"})
	cancel()
	if err != nil {
		_ = tr.emitNotice(
			harness.NoticeWarning,
			"skill_check_unavailable",
			"could not confirm the bound skill loaded: "+err.Error(),
		)
		return nil
	}
	if success, ok := response["success"].(bool); ok && !success {
		detail := response.str("error")
		if detail == "" {
			detail = "get_commands is not supported"
		}
		_ = tr.emitNotice(
			harness.NoticeWarning,
			"skill_check_unavailable",
			"could not confirm the bound skill loaded: "+detail,
		)
		return nil
	}

	want := "skill:" + name
	for _, item := range response.obj("data").list("commands") {
		command, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if record(command).str("name") == want {
			return nil
		}
	}
	return fmt.Errorf("pi did not register %q; check the skill's SKILL.md and its frontmatter name", want)
}

// answerAsk routes a Pi dialog to the host and writes the answer back.
func (a *Adapter) answerAsk(ctx context.Context, c *client, host harness.Host, ask harness.Ask) {
	// The host owns the question deadline: Ask.Timeout is a hint the host may
	// cap, extend, or replace with a configured value. Enforcing it here would
	// make a configured deadline unable to win.
	answer, err := host.Ask(ctx, ask)

	response := record{"type": "extension_ui_response", "id": ask.ID}
	switch {
	case err != nil, answer.Cancelled, ctx.Err() != nil:
		response["cancelled"] = true
	case ask.Kind == harness.AskConfirm:
		confirmed := answer.Confirmed != nil && *answer.Confirmed
		response["confirmed"] = confirmed
	default:
		response["value"] = answer.Value
	}

	_ = c.write(response)
}

// injectionPump delivers user messages into the running execution.
func (a *Adapter) injectionPump(ctx context.Context, c *client, host harness.Host, tr *translator) {
	injections := host.Injections()
	if injections == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case injection, ok := <-injections:
			if !ok {
				return
			}

			// Steering is delivered between tool calls; a follow-up waits for
			// the agent to go idle. Choosing by state is the adapter's job.
			command := "follow_up"
			if tr.isRunning() {
				command = "steer"
			}

			callCtx, cancel := context.WithTimeout(ctx, a.opts.CommandTimeout)
			response, err := c.call(callCtx, record{"type": command, "message": injection.Text})
			cancel()

			ack := &harness.InjectionAck{ID: injection.ID}
			switch {
			case err != nil:
				ack.Disposition = "failed"
				ack.Error = err.Error()
			default:
				ack.Disposition = response.obj("data").str("disposition")
				if ack.Disposition == "" {
					ack.Disposition = "queued"
				}
			}

			_ = tr.emit(harness.KindInjectionAck, func(event *harness.Event) { event.Injection = ack })
		}
	}
}

func failureResult(class harness.FailureClass, message, detail string) (harness.Result, error) {
	failure := &harness.Failure{Class: class, Message: message, Detail: detail, Retryable: isRetryable(class)}
	result := harness.Result{Status: statusFor(class), Failure: failure}
	return result, result.Err()
}

// startFailure classifies a startup that never became ready. Pi names a
// permanent configuration problem (an unknown model) and a credential problem
// in its diagnostics; anything else is an ordinary start failure that may be
// retried.
func startFailure(err error, proc *process) (harness.Result, error) {
	detail := failureDetail(err, proc)
	class, _ := classifyErrorText(detail)
	if class == harness.FailureProvider {
		class = harness.FailureStart
	}
	return failureResult(class, "pi did not become ready", detail)
}

func statusFor(class harness.FailureClass) harness.Status {
	switch class {
	case harness.FailureCancelled:
		return harness.StatusCancelled
	case harness.FailureTimeout:
		return harness.StatusTimedOut
	default:
		return harness.StatusFailed
	}
}

func isRetryable(class harness.FailureClass) bool {
	switch class {
	case harness.FailureProvider, harness.FailureStart, harness.FailureProtocol,
		harness.FailureHarnessExit, harness.FailureTimeout:
		return true
	default:
		return false
	}
}

func failureDetail(err error, proc *process) string {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	if proc != nil {
		if stderr := strings.TrimSpace(proc.stderr.String()); stderr != "" {
			detail = strings.TrimSpace(detail + "\n" + stderr)
		}
	}
	return detail
}

// promptForRequest builds the user message. With one skill it keeps Pi's own
// /skill: expansion, which also handles relative references. With several
// skills Pi only expands the first command, so the adapter inlines every skill
// body in Pi's own block shape and then appends the node prompt.
func promptForRequest(req harness.Request) (string, error) {
	skills := req.SkillList()
	if len(skills) == 0 {
		return req.Prompt, nil
	}
	if len(skills) == 1 {
		if skills[0].Name == "" {
			return req.Prompt, nil
		}
		// The explicit form forces the skill instead of hoping the model
		// picks it; Pi expands it before sending.
		return "/skill:" + skills[0].Name + "\n\n" + req.Prompt, nil
	}

	blocks := make([]string, 0, len(skills))
	for _, skill := range skills {
		block, err := skillPromptBlock(skill)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, block)
	}
	return strings.Join(blocks, "\n\n") + "\n\n" + req.Prompt, nil
}

func skillPromptBlock(skill harness.Skill) (string, error) {
	if len(skill.Paths) == 0 {
		return "", fmt.Errorf("skill %q has no path", skill.Name)
	}
	var lastErr error
	for _, path := range skill.Paths {
		file := path
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			file = filepath.Join(path, "SKILL.md")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			lastErr = err
			continue
		}
		body := stripSkillFrontmatter(string(data))
		return fmt.Sprintf(
			"<skill name=%q location=%q>\nReferences are relative to %s.\n\n%s\n</skill>",
			skill.Name,
			file,
			filepath.Dir(file),
			body,
		), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no readable SKILL.md")
	}
	return "", fmt.Errorf("read skill %q: %w", skill.Name, lastErr)
}

func stripSkillFrontmatter(body string) string {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "---") {
		return trimmed
	}
	rest := strings.TrimPrefix(trimmed, "---")
	if index := strings.Index(rest, "\n---"); index >= 0 {
		return strings.TrimSpace(rest[index+4:])
	}
	return trimmed
}

// buildArgs is the whole startup contract with Pi. It is a pure function so
// every flag combination is testable without a process.
func buildArgs(req harness.Request, cfg Config, bridge string) []string {
	args := []string{"--mode", "rpc", "--no-session", "-e", bridge}

	if cfg.Provider != "" {
		args = append(args, "--provider", cfg.Provider)
	}
	if req.Model.ID != "" {
		args = append(args, "--model", req.Model.ID)
	}
	if req.Model.Thinking != "" {
		args = append(args, "--thinking", req.Model.Thinking)
	}

	if len(req.Tools.Allow) > 0 {
		// Plain names replace the whole default selection, so the ask tool
		// has to be named explicitly or interactive nodes break.
		allow := append([]string(nil), req.Tools.Allow...)
		allow = append(allow, askToolName)
		args = append(args, "--tools", strings.Join(allow, ","))
	} else {
		args = append(args, "--tools", "+"+askToolName)
	}
	if len(req.Tools.Deny) > 0 {
		args = append(args, "--exclude-tools", strings.Join(req.Tools.Deny, ","))
	}

	skills := req.SkillList()
	skillPaths := make([]string, 0, len(skills))
	for _, skill := range skills {
		skillPaths = append(skillPaths, skill.Paths...)
	}
	if len(skillPaths) > 0 {
		// Deterministic binding: no ambient skill discovery can shadow the
		// skills the node declared.
		args = append(args, "--no-skills")
		for _, path := range skillPaths {
			args = append(args, "--skill", path)
		}
	}

	if req.Trust {
		args = append(args, "--approve")
	} else {
		args = append(args, "--no-approve")
	}

	if cfg.Offline {
		args = append(args, "--offline")
	}

	return args
}

func parseConfig(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		return Config{}, nil
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func environ(extra map[string]string) []string {
	env := os.Environ()
	if len(extra) == 0 {
		return env
	}

	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		env = append(env, key+"="+extra[key])
	}
	return env
}

var semverPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

func compareVersions(a, b string) int {
	parse := func(version string) ([3]int, bool) {
		match := semverPattern.FindStringSubmatch(version)
		if match == nil {
			return [3]int{}, false
		}
		var parts [3]int
		for i := range parts {
			value, err := strconv.Atoi(match[i+1])
			if err != nil {
				return [3]int{}, false
			}
			parts[i] = value
		}
		return parts, true
	}

	left, leftOK := parse(a)
	right, rightOK := parse(b)
	if !leftOK || !rightOK {
		return 0
	}

	for i := range left {
		switch {
		case left[i] < right[i]:
			return -1
		case left[i] > right[i]:
			return 1
		}
	}
	return 0
}

// stdio joins Pi's two pipes into one value the client can read and write.
type stdio struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (s *stdio) Read(p []byte) (int, error)  { return s.stdout.Read(p) }
func (s *stdio) Write(p []byte) (int, error) { return s.stdin.Write(p) }
func (s *stdio) Close() error {
	_ = s.stdin.Close()
	return s.stdout.Close()
}

// process is a started Pi child with a repeatable, bounded shutdown.
type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *ringBuffer

	done chan struct{}
	mu   sync.Mutex
	err  error
}

func startProcess(binary string, args []string, dir string, env []string) (*process, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr := newRingBuffer(stderrMaxBytes)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	p := &process{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.done)
	}()

	return p, nil
}

func (p *process) client() *client {
	return newClient(&stdio{stdin: p.stdin, stdout: p.stdout})
}

func (p *process) exitErr() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// stop requests an orderly shutdown, then terminates and kills. It is safe to
// call more than once and always ends with a reaped process.
func (p *process) stop(grace time.Duration) {
	_ = p.stdin.Close()

	if waitClosed(p.done, grace) {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	if waitClosed(p.done, grace) {
		return
	}
	_ = p.cmd.Process.Kill()
	<-p.done
}

func waitClosed(done <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
