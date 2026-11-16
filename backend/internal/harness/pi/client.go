package pi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"example.com/specflow/backend/internal/harness"
)

// recordMaxBytes bounds one stdout record. Tool results can be large, so the
// limit is generous; a record that exceeds it is a protocol violation, never a
// silent truncation.
const recordMaxBytes = 16 << 20

// stderrMaxBytes bounds retained diagnostics.
const stderrMaxBytes = 64 << 10

// record is one decoded JSONL record.
//
// Values keep their JSON shape (string, float64, bool, []any, map[string]any)
// because Pi's record families overlap: for example "message" is an object in
// a session event and a string in an extension dialog. Encoding the union as a
// struct would need duplicate tags; a map needs one set of accessors instead.
type record map[string]any

func (r record) str(key string) string {
	value, _ := r[key].(string)
	return value
}

func (r record) boolean(key string) bool {
	value, _ := r[key].(bool)
	return value
}

func (r record) number(key string) float64 {
	value, _ := r[key].(float64)
	return value
}

func (r record) integer(key string) int64 {
	return int64(r.number(key))
}

func (r record) obj(key string) record {
	value, _ := r[key].(map[string]any)
	if value == nil {
		return nil
	}
	return record(value)
}

func (r record) list(key string) []any {
	value, _ := r[key].([]any)
	return value
}

// encodeRecord renders one JSONL record: a single JSON object and a line feed.
func encodeRecord(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// recordQueue is an unbounded, order-preserving queue of non-response records.
//
// It must not block the reader: the reader also routes command responses, and
// a blocked reader would deadlock a caller waiting for one. Memory is bounded
// in practice by Pi's output rate versus the consumer, which is the same
// pressure the reader would apply anyway.
type recordQueue struct {
	mu     sync.Mutex
	items  []record
	wake   chan struct{}
	closed bool
}

func newRecordQueue() *recordQueue {
	return &recordQueue{wake: make(chan struct{}, 1)}
}

func (q *recordQueue) push(rec record) {
	q.mu.Lock()
	if !q.closed {
		q.items = append(q.items, rec)
	}
	q.mu.Unlock()
	q.signal()
}

func (q *recordQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.signal()
}

func (q *recordQueue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// next blocks until a record is available, the queue closes, or ctx is done.
func (q *recordQueue) next(ctx context.Context) (record, bool) {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			rec := q.items[0]
			q.items = q.items[1:]
			q.mu.Unlock()
			return rec, true
		}
		closed := q.closed
		q.mu.Unlock()

		if closed {
			return nil, false
		}

		select {
		case <-q.wake:
		case <-ctx.Done():
			return nil, false
		}
	}
}

// ringBuffer keeps the most recent bytes written to it and never blocks.
type ringBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func newRingBuffer(limit int) *ringBuffer {
	return &ringBuffer{limit: limit}
}

// Write keeps the tail of the stream, so a failure report shows the lines
// closest to the failure.
func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(p) >= r.limit {
		r.buf = append([]byte(nil), p[len(p)-r.limit:]...)
		return len(p), nil
	}

	r.buf = append(r.buf, p...)
	if len(r.buf) > r.limit {
		r.buf = append([]byte(nil), r.buf[len(r.buf)-r.limit:]...)
	}
	return len(p), nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

// client speaks Pi's RPC protocol over one stdio pair.
type client struct {
	rw      io.ReadWriteCloser
	scanner *bufio.Scanner

	writeMu sync.Mutex
	idSeq   atomic.Uint64

	mu          sync.Mutex
	pending     map[string]chan record
	protocolErr error
	readErr     error

	events *recordQueue
	done   chan struct{}
}

func newClient(rw io.ReadWriteCloser) *client {
	scanner := bufio.NewScanner(rw)
	scanner.Buffer(make([]byte, 0, 64*1024), recordMaxBytes)

	return &client{
		rw:      rw,
		scanner: scanner,
		pending: make(map[string]chan record),
		events:  newRecordQueue(),
		done:    make(chan struct{}),
	}
}

// start begins consuming stdout. It must be called before the first command so
// a fast response cannot be missed.
func (c *client) start() {
	go c.readLoop()
}

// call writes one command and waits for its correlated response.
//
// The returned error is nil for any well-formed response, including
// success:false: deciding whether a rejected command is a configuration or an
// authentication failure is the adapter's job. Transport problems — a timeout,
// a closed stream, a protocol violation — are failures here.
func (c *client) call(ctx context.Context, command record) (record, error) {
	id := fmt.Sprintf("specflow-%d", c.idSeq.Add(1))
	command["id"] = id

	responses := make(chan record, 1)
	c.mu.Lock()
	c.pending[id] = responses
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.write(command); err != nil {
		return nil, &harness.Failure{
			Class:   harness.FailureProtocol,
			Message: fmt.Sprintf("send %q command to pi", command.str("type")),
			Detail:  err.Error(),
		}
	}

	select {
	case response := <-responses:
		return response, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &harness.Failure{
				Class:   harness.FailureTimeout,
				Message: fmt.Sprintf("pi did not answer the %q command in time", command.str("type")),
			}
		}
		return nil, &harness.Failure{
			Class:   harness.FailureCancelled,
			Message: fmt.Sprintf("cancelled while waiting for the %q response", command.str("type")),
		}
	case <-c.done:
		return nil, c.exitFailure()
	}
}

func (c *client) write(value any) error {
	line, err := encodeRecord(value)
	if err != nil {
		return fmt.Errorf("encode record: %w", err)
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.rw.Write(line)
	return err
}

func (c *client) close() error {
	return c.rw.Close()
}

func (c *client) readLoop() {
	defer close(c.done)
	defer c.events.close()

	for c.scanner.Scan() {
		line := c.scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			c.setProtocolErr(fmt.Errorf("parse record: %w", err))
			return
		}

		if rec.str("type") == "response" {
			if fatal := c.routeResponse(rec); fatal {
				return
			}
			continue
		}

		c.events.push(rec)
	}

	if err := c.scanner.Err(); err != nil {
		c.mu.Lock()
		c.readErr = err
		c.mu.Unlock()
	}
}

// routeResponse delivers a command response to its waiter. It reports whether
// the stream is unusable: a command Pi could not parse means our framing or
// message shape is wrong, and continuing would only produce timeouts.
func (c *client) routeResponse(rec record) bool {
	id := rec.str("id")

	if id == "" {
		if rec.str("command") == "parse" {
			c.setProtocolErr(fmt.Errorf("pi rejected a command: %s", rec.str("error")))
			return true
		}
		return false
	}

	c.mu.Lock()
	responses := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()

	if responses == nil {
		// A response for a command that already timed out.
		return false
	}
	responses <- rec
	return false
}

func (c *client) setProtocolErr(err error) {
	c.mu.Lock()
	c.protocolErr = err
	c.mu.Unlock()
}

func (c *client) protocolFailure() *harness.Failure {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.protocolErr == nil {
		return nil
	}
	return &harness.Failure{
		Class:     harness.FailureProtocol,
		Message:   "pi stopped speaking its protocol",
		Detail:    c.protocolErr.Error(),
		Retryable: true,
	}
}

func (c *client) readFailure() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
}

func (c *client) cause() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.protocolErr != nil {
		return c.protocolErr
	}
	return c.readErr
}

func (c *client) exitFailure() *harness.Failure {
	if err := c.cause(); err != nil {
		return &harness.Failure{
			Class:     harness.FailureProtocol,
			Message:   "pi stopped speaking its protocol",
			Detail:    err.Error(),
			Retryable: true,
		}
	}
	return &harness.Failure{
		Class:     harness.FailureHarnessExit,
		Message:   "pi exited",
		Retryable: true,
	}
}
