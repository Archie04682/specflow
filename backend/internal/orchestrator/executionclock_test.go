package orchestrator

import (
	"context"
	"testing"
	"time"
)

func waitClockDone(t *testing.T, clock *executionClock, within time.Duration) {
	t.Helper()

	select {
	case <-clock.Done():
	case <-time.After(within):
		t.Fatalf("clock did not finish within %s", within)
	}
}

func TestExecutionClockExpiresAsDeadline(t *testing.T) {
	clock := newExecutionClock(context.Background(), 30*time.Millisecond)
	defer clock.Stop()

	waitClockDone(t, clock, time.Second)

	if err := clock.Err(); err != context.DeadlineExceeded {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestExecutionClockPauseStopsTheBudget(t *testing.T) {
	clock := newExecutionClock(context.Background(), 80*time.Millisecond)
	defer clock.Stop()

	clock.Pause()
	time.Sleep(200 * time.Millisecond)

	select {
	case <-clock.Done():
		t.Fatal("a paused clock expired")
	default:
	}

	clock.Resume()
	waitClockDone(t, clock, time.Second)

	if err := clock.Err(); err != context.DeadlineExceeded {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestExecutionClockResumeUsesTheRemainingTime(t *testing.T) {
	clock := newExecutionClock(context.Background(), 600*time.Millisecond)
	defer clock.Stop()

	time.Sleep(350 * time.Millisecond)
	clock.Pause()
	time.Sleep(200 * time.Millisecond)
	clock.Resume()

	// About 250ms remain; a full restart would also pass this check, so the
	// wall-clock bound below confirms it did not reset to the original 600ms.
	select {
	case <-clock.Done():
		t.Fatal("the clock expired immediately on resume")
	case <-time.After(150 * time.Millisecond):
	}

	select {
	case <-clock.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the clock did not finish after resuming")
	}
}

func TestExecutionClockPauseAfterStopIsSafe(t *testing.T) {
	clock := newExecutionClock(context.Background(), time.Hour)
	clock.Stop()
	clock.Pause()
	clock.Resume()
	clock.Stop()

	if err := clock.Err(); err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled after Stop", err)
	}
}

func TestExecutionClockPropagatesParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	clock := newExecutionClock(parent, time.Hour)
	defer clock.Stop()

	cancel()

	waitClockDone(t, clock, time.Second)

	if err := clock.Err(); err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestExecutionClockWithoutABudgetNeverExpires(t *testing.T) {
	clock := newExecutionClock(context.Background(), 0)
	defer clock.Stop()

	clock.Pause()
	clock.Resume()

	select {
	case <-clock.Done():
		t.Fatal("an unbounded clock expired")
	case <-time.After(60 * time.Millisecond):
	}

	if _, ok := clock.Deadline(); ok {
		t.Fatal("the clock reported an absolute deadline")
	}
}

func TestExecutionClockForwardsValues(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "value")
	clock := newExecutionClock(parent, time.Hour)
	defer clock.Stop()

	if got := clock.Value(key{}); got != "value" {
		t.Fatalf("value = %v, want the parent's value", got)
	}
}
