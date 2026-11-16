package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
)

type questionEvent struct {
	ask      harness.Ask
	deadline time.Time
}

type resolutionEvent struct {
	id         string
	resolution Resolution
	answer     harness.Answer
}

type recordingSink struct {
	questions chan questionEvent
	resolved  chan resolutionEvent
	emitted   chan harness.Event
	queued    chan harness.Injection
}

func newRecordingSink() *recordingSink {
	return &recordingSink{
		questions: make(chan questionEvent, 8),
		resolved:  make(chan resolutionEvent, 8),
		emitted:   make(chan harness.Event, 8),
		queued:    make(chan harness.Injection, 512),
	}
}

func (s *recordingSink) Emit(event harness.Event) error {
	s.emitted <- event
	return nil
}

func (s *recordingSink) Question(ask harness.Ask, deadline time.Time) {
	s.questions <- questionEvent{ask: ask, deadline: deadline}
}

func (s *recordingSink) Resolved(questionID string, resolution Resolution, answer harness.Answer) {
	s.resolved <- resolutionEvent{id: questionID, resolution: resolution, answer: answer}
}

func (s *recordingSink) InjectionQueued(injection harness.Injection) {
	s.queued <- injection
}

func newTestSession(sink Sink, askTimeout time.Duration) *Session {
	return New("node", sink, askTimeout)
}

func awaitAsk(t *testing.T, sink *recordingSink) questionEvent {
	t.Helper()
	select {
	case event := <-sink.questions:
		return event
	case <-time.After(time.Second):
		t.Fatal("the question was never surfaced")
		return questionEvent{}
	}
}

func awaitResolution(t *testing.T, sink *recordingSink) resolutionEvent {
	t.Helper()
	select {
	case event := <-sink.resolved:
		return event
	case <-time.After(time.Second):
		t.Fatal("the question resolution was never reported")
		return resolutionEvent{}
	}
}

func TestAskBlocksUntilAnsweredAndDiscardsLateAnswers(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	answers := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1", Kind: harness.AskInput})
		answers <- answer
	}()

	question := awaitAsk(t, sink)
	if question.ask.ID != "q1" {
		t.Fatalf("ask id = %q", question.ask.ID)
	}
	if !question.deadline.IsZero() {
		t.Fatalf("default deadline = %v, want no deadline", question.deadline)
	}

	if status := summary.Answer("q1", harness.Answer{ID: "q1", Value: "yes"}); status != AnswerDelivered {
		t.Fatalf("answer status = %v", status)
	}
	answer := <-answers
	if answer.Value != "yes" {
		t.Fatalf("answer = %+v", answer)
	}
	if resolved := awaitResolution(t, sink); resolved.id != "q1" || resolved.resolution != ResolutionAnswered {
		t.Fatalf("resolution = %+v", resolved)
	}

	if status := summary.Answer("q1", harness.Answer{ID: "q1", Value: "late"}); status != AnswerResolved {
		t.Fatalf("late answer status = %v", status)
	}
}

func TestAnswerUnknownQuestion(t *testing.T) {
	summary := newTestSession(newRecordingSink(), 0)
	if status := summary.Answer("missing", harness.Answer{ID: "missing"}); status != AnswerUnknown {
		t.Fatalf("status = %v", status)
	}
}

func TestAskTimesOutAndReportsTheReason(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	answers := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1", Timeout: 20 * time.Millisecond})
		answers <- answer
	}()

	awaitAsk(t, sink)
	answer := <-answers
	if !answer.Cancelled {
		t.Fatalf("answer = %+v, want cancelled", answer)
	}
	if resolved := awaitResolution(t, sink); resolved.resolution != ResolutionTimedOut {
		t.Fatalf("resolution = %+v, want timed out", resolved)
	}
	if status := summary.Answer("q1", harness.Answer{ID: "q1"}); status != AnswerResolved {
		t.Fatalf("answer after timeout = %v, want resolved", status)
	}
}

func TestConfiguredDeadlineWinsOverTheHarnessHint(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 40*time.Millisecond)

	done := make(chan harness.Answer, 1)
	start := time.Now()
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1", Timeout: 10 * time.Second})
		done <- answer
	}()

	question := awaitAsk(t, sink)
	if remaining := time.Until(question.deadline); remaining > time.Second {
		t.Fatalf("deadline = %s, want the configured 40ms rather than the 10s hint", remaining)
	}
	answer := <-done
	if !answer.Cancelled {
		t.Fatalf("answer = %+v", answer)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the configured deadline was not enforced: %s", elapsed)
	}
}

func TestHarnessHintAppliesWhenNothingIsConfigured(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	done := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1", Timeout: 30 * time.Millisecond})
		done <- answer
	}()

	question := awaitAsk(t, sink)
	if remaining := time.Until(question.deadline); remaining > time.Second {
		t.Fatalf("deadline = %s, want the harness hint", remaining)
	}
	<-done
}

func TestSecondQuestionWaitsForTheFirst(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	first := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1"})
		first <- answer
	}()
	if question := awaitAsk(t, sink); question.ask.ID != "q1" {
		t.Fatalf("first question = %q", question.ask.ID)
	}

	second := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q2"})
		second <- answer
	}()

	select {
	case question := <-sink.questions:
		t.Fatalf("the second question was surfaced before the first resolved: %q", question.ask.ID)
	case <-time.After(50 * time.Millisecond):
	}

	if status := summary.Answer("q1", harness.Answer{ID: "q1", Value: "one"}); status != AnswerDelivered {
		t.Fatalf("first answer status = %v", status)
	}
	<-first
	awaitResolution(t, sink)

	if question := awaitAsk(t, sink); question.ask.ID != "q2" {
		t.Fatalf("second question = %q", question.ask.ID)
	}
	if status := summary.Answer("q2", harness.Answer{ID: "q2", Value: "two"}); status != AnswerDelivered {
		t.Fatalf("second answer status = %v", status)
	}
	<-second
}

func TestDuplicateQuestionIDIsRejected(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	answers := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1"})
		answers <- answer
	}()
	awaitAsk(t, sink)
	summary.Answer("q1", harness.Answer{ID: "q1"})
	<-answers
	awaitResolution(t, sink)

	if _, err := summary.Ask(context.Background(), harness.Ask{ID: "q1"}); err == nil {
		t.Fatal("reusing a question id was accepted")
	}
}

func TestAskEndsWhenTheExecutionIsCancelled(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	ctx, cancel := context.WithCancel(context.Background())
	answers := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(ctx, harness.Ask{ID: "q1"})
		answers <- answer
	}()

	awaitAsk(t, sink)
	cancel()

	answer := <-answers
	if !answer.Cancelled {
		t.Fatalf("answer = %+v", answer)
	}
	if resolved := awaitResolution(t, sink); resolved.resolution != ResolutionCancelled {
		t.Fatalf("resolution = %+v, want cancelled", resolved)
	}
}

func TestCloseReleasesAPendingQuestion(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	answers := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1"})
		answers <- answer
	}()

	awaitAsk(t, sink)
	summary.Close()

	answer := <-answers
	if !answer.Cancelled {
		t.Fatalf("answer = %+v", answer)
	}
}

func TestInjectAccumulatesInOrder(t *testing.T) {
	summary := newTestSession(newRecordingSink(), 0)

	for _, text := range []string{"one", "two", "three"} {
		if _, err := summary.Inject(context.Background(), text); err != nil {
			t.Fatalf("inject %q: %v", text, err)
		}
	}

	for _, want := range []string{"one", "two", "three"} {
		select {
		case injection := <-summary.Injections():
			if injection.Text != want {
				t.Fatalf("injection = %q, want %q", injection.Text, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("injection %q was never readable", want)
		}
	}
}

func TestInjectIsNonBlockingWhenTheQueueIsFull(t *testing.T) {
	summary := newTestSession(newRecordingSink(), 0)

	for index := 0; index < injectionQueueSize; index++ {
		if _, err := summary.Inject(context.Background(), "message"); err != nil {
			t.Fatalf("inject %d: %v", index, err)
		}
	}

	start := time.Now()
	_, err := summary.Inject(context.Background(), "overflow")
	if !errors.Is(err, ErrInjectionQueueFull) {
		t.Fatalf("err = %v, want ErrInjectionQueueFull", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("a full queue blocked for %s", elapsed)
	}
}

func TestInjectAfterCloseIsRejectedEvenWithAFullQueue(t *testing.T) {
	summary := newTestSession(newRecordingSink(), 0)
	for index := 0; index < injectionQueueSize; index++ {
		if _, err := summary.Inject(context.Background(), "message"); err != nil {
			t.Fatal(err)
		}
	}
	summary.Close()

	if _, err := summary.Inject(context.Background(), "too late"); !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

func TestEmitForwardsTelemetry(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	if err := summary.Emit(harness.Event{Kind: harness.KindTextDelta, Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	event := <-sink.emitted
	if event.Kind != harness.KindTextDelta || event.Text != "hi" {
		t.Fatalf("event = %+v", event)
	}
}

func TestCancelledAnswerReportsCancellation(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	answers := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1"})
		answers <- answer
	}()
	awaitAsk(t, sink)

	if status := summary.Answer("q1", harness.Answer{ID: "q1", Cancelled: true}); status != AnswerDelivered {
		t.Fatalf("status = %v", status)
	}
	if answer := <-answers; !answer.Cancelled {
		t.Fatalf("answer = %+v, want cancelled", answer)
	}
	if resolved := awaitResolution(t, sink); resolved.resolution != ResolutionCancelled {
		t.Fatalf("resolution = %+v, want cancelled", resolved)
	}
}

func TestNoConfiguredDeadlineWaitsUntilAnswered(t *testing.T) {
	sink := newRecordingSink()
	summary := newTestSession(sink, 0)

	answers := make(chan harness.Answer, 1)
	go func() {
		answer, _ := summary.Ask(context.Background(), harness.Ask{ID: "q1"})
		answers <- answer
	}()

	asked := awaitAsk(t, sink)
	if !asked.deadline.IsZero() {
		t.Fatalf("deadline = %v, want no deadline", asked.deadline)
	}

	select {
	case <-answers:
		t.Fatal("an unbounded question returned before it was answered")
	case <-time.After(60 * time.Millisecond):
	}

	if status := summary.Answer("q1", harness.Answer{ID: "q1", Value: "yes"}); status != AnswerDelivered {
		t.Fatalf("answer status = %v", status)
	}
	if answer := <-answers; answer.Value != "yes" {
		t.Fatalf("answer = %+v", answer)
	}
	if resolved := awaitResolution(t, sink); resolved.resolution != ResolutionAnswered {
		t.Fatalf("resolution = %+v", resolved)
	}
}
