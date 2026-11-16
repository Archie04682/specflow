// Package mock provides a scripted harness adapter for tests.
//
// It exercises the contract without a runtime: events in a fixed order, an
// optional question, an optional wait for an injection, and a canned terminal
// result. It is the adapter the orchestrator and UI can develop against before
// a real harness is available.
package mock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"example.com/specflow/backend/internal/harness"
)

// DescriptorID is the identifier the mock registers under.
const DescriptorID = "mock"

// Script is what a mock execution does.
type Script struct {
	// Events are emitted in order before any question or injection wait.
	Events []harness.Event
	// Writes are files created in the execution's working directory, relative
	// to it. WritesOn limits them to one node, so a scripted flow can create a
	// change only at the node that proposes it.
	Writes   map[string]string
	WritesOn string
	// Ask, when set, is asked after the events.
	Ask *harness.Ask
	// WaitForInjection makes the execution wait for one injected message and
	// acknowledge it.
	WaitForInjection bool
	// EmitDelay slows every Emit, for backpressure tests.
	EmitDelay time.Duration
	// Result is the terminal outcome. Its Status defaults to completed.
	Result harness.Result
}

// Adapter is a scripted harness.Adapter.
type Adapter struct {
	script Script
}

// New returns a mock adapter that runs the script.
func New(script Script) *Adapter {
	return &Adapter{script: script}
}

// Descriptor identifies the mock.
func (a *Adapter) Descriptor() harness.Descriptor {
	return harness.Descriptor{
		ID:       DescriptorID,
		Name:     "Mock",
		Version:  "0.1.0",
		Protocol: harness.ProtocolVersion,
	}
}

// Capabilities reports everything on, so the mock can stand in for any node.
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

// Probe reports the mock as ready.
func (a *Adapter) Probe(context.Context) (harness.ProbeResult, error) {
	return harness.ProbeResult{
		RuntimeVersion: "mock",
		Capabilities:   a.Capabilities(),
	}, nil
}

// Run executes the script once. The answered value becomes the result text so
// callers can observe which answer the adapter received.
func (a *Adapter) Run(ctx context.Context, request harness.Request, host harness.Host) (harness.Result, error) {
	result := a.script.Result
	if result.Status == "" {
		result.Status = harness.StatusCompleted
	}

	if err := a.applyWrites(request); err != nil {
		return failed(result, harness.FailureConfig, err.Error())
	}

	for i := range a.script.Events {
		if err := ctx.Err(); err != nil {
			return cancelled(result, "context cancelled before events finished")
		}

		event := a.script.Events[i]
		if a.script.EmitDelay > 0 {
			time.Sleep(a.script.EmitDelay)
		}
		event.Seq = uint64(i + 1)
		if event.At.IsZero() {
			event.At = time.Now()
		}

		if err := host.Emit(event); err != nil {
			return failed(result, harness.FailureProtocol, "emit event: "+err.Error())
		}
	}

	if a.script.Ask != nil {
		answer, err := host.Ask(ctx, *a.script.Ask)
		if err != nil {
			return cancelled(result, "ask failed: "+err.Error())
		}
		if answer.Cancelled {
			return cancelled(result, "question cancelled")
		}
		result.Text = answer.Value
	}

	if a.script.WaitForInjection {
		select {
		case injection := <-host.Injections():
			ack := harness.Event{
				Seq:       uint64(len(a.script.Events) + 1),
				At:        time.Now(),
				Kind:      harness.KindInjectionAck,
				Injection: &harness.InjectionAck{ID: injection.ID, Disposition: "delivered"},
			}
			if err := host.Emit(ack); err != nil {
				return failed(result, harness.FailureProtocol, "acknowledge injection: "+err.Error())
			}
		case <-ctx.Done():
			return cancelled(result, "context cancelled while waiting for an injection")
		}
	}

	return result, result.Err()
}

// applyWrites creates the script's files when the request names the node the
// script pins them to. A script with writes but no workdir is a test bug, not
// a silent write into the process directory.
func (a *Adapter) applyWrites(request harness.Request) error {
	if len(a.script.Writes) == 0 {
		return nil
	}
	if a.script.WritesOn != "" && a.script.WritesOn != request.NodeID {
		return nil
	}
	if request.Workdir == "" {
		return errors.New("script writes files but the request has no workdir")
	}

	for name, content := range a.script.Writes {
		path := filepath.Join(request.Workdir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func failed(result harness.Result, class harness.FailureClass, message string) (harness.Result, error) {
	result.Status = harness.StatusFailed
	result.Failure = &harness.Failure{Class: class, Message: message}
	return result, result.Err()
}

func cancelled(result harness.Result, message string) (harness.Result, error) {
	result.Status = harness.StatusCancelled
	result.Failure = &harness.Failure{Class: harness.FailureCancelled, Message: message}
	return result, result.Err()
}
