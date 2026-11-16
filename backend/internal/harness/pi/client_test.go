package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type fakeStdio struct {
	in  io.ReadCloser
	out io.Writer
}

func (f *fakeStdio) Read(p []byte) (int, error)  { return f.in.Read(p) }
func (f *fakeStdio) Write(p []byte) (int, error) { return f.out.Write(p) }
func (f *fakeStdio) Close() error                { return f.in.Close() }

// newTestClient wires a client to an in-memory stdin/stdout pair.
func newTestClient(t *testing.T) (*client, *io.PipeWriter, *syncBuffer) {
	t.Helper()

	reader, writer := io.Pipe()
	written := &syncBuffer{}
	c := newClient(&fakeStdio{in: reader, out: written})
	c.start()

	t.Cleanup(func() {
		_ = writer.Close()
		_ = c.close()
	})

	return c, writer, written
}

func sendLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatalf("write line: %v", err)
	}
}

func nextEvent(t *testing.T, c *client) record {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rec, ok := c.events.next(ctx)
	if !ok {
		t.Fatal("no event received")
	}
	return rec
}

// waitForCommand blocks until the client has written a command of the given
// type, and returns its decoded JSON.
func waitForCommand(t *testing.T, buf *syncBuffer, kind string) map[string]any {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, line := range strings.Split(buf.String(), "\n") {
			if line == "" {
				continue
			}
			var command map[string]any
			if err := json.Unmarshal([]byte(line), &command); err != nil {
				t.Fatalf("decode written command %q: %v", line, err)
			}
			if command["type"] == kind {
				return command
			}
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("command %q was never written; got %q", kind, buf.String())
	return nil
}

func TestRecordsSplitOnLineFeedOnly(t *testing.T) {
	c, writer, _ := newTestClient(t)

	sendLine(t, writer, `{"type":"first"}`)
	if got := nextEvent(t, c).str("type"); got != "first" {
		t.Fatalf("type = %q, want first", got)
	}

	sendLine(t, writer, `{"type":"second"}`)
	if got := nextEvent(t, c).str("type"); got != "second" {
		t.Fatalf("type = %q, want second", got)
	}
}

func TestRecordsTolerateCarriageReturn(t *testing.T) {
	c, writer, _ := newTestClient(t)

	if _, err := io.WriteString(writer, "{\"type\":\"crlf\"}\r\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := nextEvent(t, c).str("type"); got != "crlf" {
		t.Fatalf("type = %q, want crlf", got)
	}
}

// TestUnicodeSeparatorsInsideStringsAreNotRecordBoundaries pins the framing
// requirement: JSON strings may contain U+2028 and U+2029, and they are data,
// not delimiters.
func TestUnicodeSeparatorsInsideStringsAreNotRecordBoundaries(t *testing.T) {
	c, writer, _ := newTestClient(t)

	payload := "{\"type\":\"one\",\"text\":\"a\u2028b\u2029c\"}"
	if _, err := io.WriteString(writer, payload+"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec := nextEvent(t, c)
	if got := rec.str("type"); got != "one" {
		t.Fatalf("type = %q, want one (the record was split)", got)
	}
	if got, want := rec.str("text"), "a\u2028b\u2029c"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}

func TestWriteTerminatesRecordsWithLineFeed(t *testing.T) {
	c, _, written := newTestClient(t)

	if err := c.write(map[string]any{"type": "hello"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	if !strings.HasSuffix(written.String(), "\n") {
		t.Fatalf("written %q, want a trailing line feed", written.String())
	}
	if strings.Count(written.String(), "\n") != 1 {
		t.Fatalf("written %q, want exactly one record", written.String())
	}
}

func TestCallCorrelatesByID(t *testing.T) {
	c, writer, written := newTestClient(t)

	result := make(chan error, 1)
	go func() {
		_, err := c.call(context.Background(), record{"type": "get_state"})
		result <- err
	}()

	command := waitForCommand(t, written, "get_state")
	id, _ := command["id"].(string)
	if id == "" {
		t.Fatal("command was written without an id")
	}

	sendLine(t, writer, `{"id":"`+id+`","type":"response","command":"get_state","success":true,"data":{"sessionId":"s1"}}`)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not return")
	}
}

func TestCallIgnoresUnknownAndUncorrelatedResponses(t *testing.T) {
	c, writer, written := newTestClient(t)

	result := make(chan error, 1)
	go func() {
		_, err := c.call(context.Background(), record{"type": "get_state"})
		result <- err
	}()

	command := waitForCommand(t, written, "get_state")
	id, _ := command["id"].(string)

	sendLine(t, writer, `{"id":"not-this-one","type":"response","command":"get_state","success":true}`)
	sendLine(t, writer, `{"type":"response","command":"other","success":true}`)
	sendLine(t, writer, `{"id":"`+id+`","type":"response","command":"get_state","success":true,"data":{"sessionId":"s1"}}`)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not return")
	}
}

func TestParseFailureWithoutIDIsProtocolError(t *testing.T) {
	c, writer, _ := newTestClient(t)

	sendLine(t, writer, `{"type":"response","command":"parse","success":false,"error":"Unexpected token"}`)

	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("read loop did not stop after a parse failure")
	}

	if err := c.cause(); err == nil {
		t.Fatal("cause() = nil, want a protocol error")
	}
}

func TestCallTimesOut(t *testing.T) {
	c, _, _ := newTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := c.call(ctx, record{"type": "get_state"})
	if err == nil {
		t.Fatal("call returned nil error, want a timeout")
	}

	var failure *harness.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T, want *harness.Failure", err)
	}
	if failure.Class != harness.FailureTimeout {
		t.Fatalf("class = %q, want %q", failure.Class, harness.FailureTimeout)
	}
}

func TestCallFailsWhenStreamCloses(t *testing.T) {
	c, writer, _ := newTestClient(t)

	result := make(chan error, 1)
	go func() {
		_, err := c.call(context.Background(), record{"type": "get_state"})
		result <- err
	}()

	time.Sleep(10 * time.Millisecond)
	_ = writer.Close()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("call returned nil error after the stream closed")
		}
		var failure *harness.Failure
		if !errors.As(err, &failure) {
			t.Fatalf("error = %T, want *harness.Failure", err)
		}
		if failure.Class != harness.FailureHarnessExit && failure.Class != harness.FailureProtocol {
			t.Fatalf("class = %q, want a harness exit or protocol failure", failure.Class)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not return after the stream closed")
	}
}

func TestLargeRecordIsReadIntact(t *testing.T) {
	c, writer, _ := newTestClient(t)

	payload := strings.Repeat("x", 4<<20)
	encoded, err := json.Marshal(map[string]any{"type": "big", "text": payload})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	go func() {
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			t.Errorf("write: %v", err)
		}
	}()

	rec := nextEvent(t, c)
	if got := len(rec.str("text")); got != len(payload) {
		t.Fatalf("text length = %d, want %d (the record was truncated)", got, len(payload))
	}
}

func TestStderrRingKeepsTheTail(t *testing.T) {
	ring := newRingBuffer(1024)

	total := 200 * 1024
	for written := 0; written < total; written += 512 {
		chunk := bytes.Repeat([]byte("a"), 512)
		if written+512 >= total {
			chunk = bytes.Repeat([]byte("z"), 512)
		}
		if _, err := ring.Write(chunk); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	got := ring.String()
	if len(got) > 1024 {
		t.Fatalf("ring holds %d bytes, want at most 1024", len(got))
	}
	if !strings.HasSuffix(got, strings.Repeat("z", 512)) {
		t.Fatalf("ring does not end with the most recent bytes: %q", got[max(0, len(got)-32):])
	}
}
