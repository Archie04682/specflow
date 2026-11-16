// Package session implements the orchestrator's side of a harness execution:
// it turns telemetry into run events, holds a node while its harness asks the
// user a question, and feeds mid-run clarifications to the adapter.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"example.com/specflow/backend/internal/harness"
)

// injectionQueueSize bounds the clarification buffer. Injection is
// non-blocking; a full queue is rejected rather than waiting for the adapter.
const injectionQueueSize = 256

// ErrClosed reports a session whose execution has finished.
var ErrClosed = errors.New("session is closed")

// ErrInjectionQueueFull reports a clarification the session could not queue.
var ErrInjectionQueueFull = errors.New("injection queue is full")

// Resolution is why a pending question stopped waiting.
type Resolution string

const (
	// ResolutionAnswered means the user answered before the deadline.
	ResolutionAnswered Resolution = "answered"
	// ResolutionTimedOut means the question reached its deadline.
	ResolutionTimedOut Resolution = "timed_out"
	// ResolutionCancelled means the execution or run was cancelled.
	ResolutionCancelled Resolution = "cancelled"
)

// AnswerStatus reports what happened to an answer submitted to the session.
type AnswerStatus int

const (
	// AnswerDelivered means the answer won the race and reached the execution.
	AnswerDelivered AnswerStatus = iota
	// AnswerUnknown means no question with that identifier was ever asked.
	AnswerUnknown
	// AnswerResolved means the question had already stopped waiting.
	AnswerResolved
)

// Sink receives what a session observes.
type Sink interface {
	// Emit forwards one telemetry event.
	Emit(harness.Event) error
	// Question reports that the harness is waiting for an answer and the
	// deadline the host will enforce.
	Question(ask harness.Ask, deadline time.Time)
	// Resolved reports why a question stopped waiting and, when it was
	// answered, the answer that was delivered.
	Resolved(questionID string, resolution Resolution, answer harness.Answer)
	// InjectionQueued reports a clarification that is about to become
	// readable by the adapter. It is called before the message is enqueued,
	// so the run can record it before the execution acknowledges it.
	InjectionQueued(injection harness.Injection)
}

// pendingQuestion is one question's resolution state. Entries are retained for
// the session's lifetime so a late answer is reported as resolved instead of
// unknown.
type pendingQuestion struct {
	ask      harness.Ask
	channel  chan harness.Answer
	resolved bool
}

// Session is the harness.Host for one node execution.
type Session struct {
	nodeID     string
	sink       Sink
	askTimeout time.Duration

	mu        sync.Mutex
	pending   map[string]*pendingQuestion
	injectSeq uint64
	closed    bool

	askSlot    chan struct{}
	injections chan harness.Injection
	done       chan struct{}
}

// New returns a session for one node execution. askTimeout is the configured
// question deadline; zero means the plan did not configure one.
func New(nodeID string, sink Sink, askTimeout time.Duration) *Session {
	return &Session{
		nodeID:     nodeID,
		sink:       sink,
		askTimeout: askTimeout,
		pending:    make(map[string]*pendingQuestion),
		askSlot:    make(chan struct{}, 1),
		injections: make(chan harness.Injection, injectionQueueSize),
		done:       make(chan struct{}),
	}
}

// Emit forwards telemetry. The adapter sees errors raised by the sink.
func (s *Session) Emit(event harness.Event) error {
	return s.sink.Emit(event)
}

// Ask blocks until the question is answered, cancelled, or reaches its
// deadline. Only one question is active at a time; a later question waits for
// the earlier one to resolve. The deadline starts when the question becomes
// active, not while it is queued. A zero deadline means the run waits for the
// operator: only a configured ask timeout or the harness's suggested timeout
// bounds the wait.
func (s *Session) Ask(ctx context.Context, ask harness.Ask) (harness.Answer, error) {
	select {
	case s.askSlot <- struct{}{}:
	case <-ctx.Done():
		return cancelledAnswer(ask.ID), nil
	case <-s.done:
		return cancelledAnswer(ask.ID), nil
	}
	defer func() { <-s.askSlot }()

	timeout := s.askTimeout
	if timeout <= 0 {
		timeout = ask.Timeout
	}

	channel := make(chan harness.Answer, 1)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return cancelledAnswer(ask.ID), nil
	}
	if _, exists := s.pending[ask.ID]; exists {
		s.mu.Unlock()
		return cancelledAnswer(ask.ID), fmt.Errorf("question %q is already known to this session", ask.ID)
	}
	s.pending[ask.ID] = &pendingQuestion{ask: ask, channel: channel}
	s.mu.Unlock()

	var timer *time.Timer
	deadline := time.Time{}
	var timerC <-chan time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
		timer = time.NewTimer(timeout)
		timerC = timer.C
		defer timer.Stop()
	}

	s.sink.Question(ask, deadline)

	select {
	case answer := <-channel:
		return answer, nil
	case <-timerC:
		return s.cancelQuestion(ask.ID, ResolutionTimedOut)
	case <-ctx.Done():
		return s.cancelQuestion(ask.ID, ResolutionCancelled)
	case <-s.done:
		return s.cancelQuestion(ask.ID, ResolutionCancelled)
	}
}

// cancelQuestion resolves a question the waiting Ask lost. When an answer won
// the race instead, the answer is read back and returned.
func (s *Session) cancelQuestion(questionID string, resolution Resolution) (harness.Answer, error) {
	if s.resolveIfPending(questionID, resolution) {
		return cancelledAnswer(questionID), nil
	}

	s.mu.Lock()
	question := s.pending[questionID]
	s.mu.Unlock()
	if question == nil {
		return cancelledAnswer(questionID), nil
	}
	return <-question.channel, nil
}

// resolveIfPending marks a question resolved and reports the resolution. It
// reports false when another path already resolved the question.
func (s *Session) resolveIfPending(questionID string, resolution Resolution) bool {
	s.mu.Lock()
	question := s.pending[questionID]
	if question == nil || question.resolved {
		s.mu.Unlock()
		return false
	}
	question.resolved = true
	s.mu.Unlock()

	s.sink.Resolved(questionID, resolution, harness.Answer{})
	return true
}

// Answer resolves a pending question. It reports whether the answer was
// delivered, the question was unknown, or the question had already resolved.
func (s *Session) Answer(questionID string, answer harness.Answer) AnswerStatus {
	s.mu.Lock()
	question := s.pending[questionID]
	if question == nil {
		s.mu.Unlock()
		return AnswerUnknown
	}
	if s.closed || question.resolved {
		s.mu.Unlock()
		return AnswerResolved
	}
	question.resolved = true
	s.mu.Unlock()

	resolution := ResolutionAnswered
	if answer.Cancelled {
		resolution = ResolutionCancelled
	}
	// Report the transition before waking the adapter, so the run event is
	// written before the execution can continue.
	s.sink.Resolved(questionID, resolution, answer)
	question.channel <- answer
	return AnswerDelivered
}

// Inject queues a clarification for the running execution. It never waits for
// the adapter: a full queue is reported immediately.
func (s *Session) Inject(ctx context.Context, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrClosed
	}
	if len(s.injections) >= cap(s.injections) {
		return "", ErrInjectionQueueFull
	}

	s.injectSeq++
	id := fmt.Sprintf("%s-%d", s.nodeID, s.injectSeq)
	injection := harness.Injection{ID: id, Text: text}
	// Report before enqueueing: the adapter can read the moment the send
	// completes, and its acknowledgement must not precede the received event.
	s.sink.InjectionQueued(injection)
	s.injections <- injection
	return id, nil
}

// Injections is the channel the adapter reads clarifications from.
func (s *Session) Injections() <-chan harness.Injection {
	return s.injections
}

// Pending reports whether an unresolved question is outstanding.
func (s *Session) Pending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, question := range s.pending {
		if !question.resolved {
			return true
		}
	}
	return false
}

// Close ends the session and releases anything blocked on it.
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.done)
	s.mu.Unlock()
}

func cancelledAnswer(questionID string) harness.Answer {
	return harness.Answer{ID: questionID, Cancelled: true}
}
