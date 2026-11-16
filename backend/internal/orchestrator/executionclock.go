package orchestrator

import (
	"context"
	"sync"
	"time"
)

// executionClock bounds an agent node's active execution time.
//
// Unlike context.WithTimeout it can pause: while the run waits on the operator
// the remaining budget does not elapse, so a question never spends the node's
// timeout. It implements context.Context itself, and an expired budget reports
// context.DeadlineExceeded from Err, which is what the harness adapters use to
// classify a timeout.
type executionClock struct {
	parent context.Context

	mu        sync.Mutex
	budgeted  bool
	remaining time.Duration
	started   time.Time
	running   bool
	stopped   bool
	timer     *time.Timer

	done chan struct{}
	err  error
	once sync.Once
}

// newExecutionClock starts bounding parent with timeout. A non-positive
// timeout means the clock never expires; it still propagates cancellation.
func newExecutionClock(parent context.Context, timeout time.Duration) *executionClock {
	clock := &executionClock{
		parent:    parent,
		budgeted:  timeout > 0,
		remaining: timeout,
		started:   time.Now(),
		running:   timeout > 0,
		done:      make(chan struct{}),
	}
	if clock.budgeted {
		clock.timer = time.AfterFunc(timeout, clock.expire)
	}
	go clock.watchParent()
	return clock
}

// Deadline reports no absolute deadline: the budget is active time, so a
// wall-clock deadline would be wrong. Callers that need one derive their own.
func (c *executionClock) Deadline() (time.Time, bool) { return time.Time{}, false }

func (c *executionClock) Done() <-chan struct{} { return c.done }

func (c *executionClock) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *executionClock) Value(key any) any { return c.parent.Value(key) }

// Pause banks the elapsed active time and stops the budget. It is safe to call
// more than once and after the execution ended.
func (c *executionClock) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.budgeted || c.stopped || !c.running {
		return
	}
	c.remaining -= time.Since(c.started)
	if c.remaining < 0 {
		c.remaining = 0
	}
	c.running = false
	if c.timer != nil {
		c.timer.Stop()
	}
}

// Resume restarts the budget from the remaining active time.
func (c *executionClock) Resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.budgeted || c.stopped || c.running {
		return
	}
	c.running = true
	c.started = time.Now()
	c.timer = time.AfterFunc(c.remaining, c.expire)
}

// Stop finishes the clock when the execution ends. It reports cancellation so
// Done is closed exactly once and the parent watcher exits; nothing reads the
// clock after the adapter returns.
func (c *executionClock) Stop() { c.finish(context.Canceled) }

func (c *executionClock) expire() {
	c.mu.Lock()
	if c.stopped || !c.running {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	c.running = false
	c.err = context.DeadlineExceeded
	c.mu.Unlock()
	c.close()
}

func (c *executionClock) finish(err error) {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	c.running = false
	c.err = err
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
	c.close()
}

func (c *executionClock) close() {
	c.once.Do(func() { close(c.done) })
}

func (c *executionClock) watchParent() {
	select {
	case <-c.parent.Done():
		c.finish(c.parent.Err())
	case <-c.done:
	}
}
