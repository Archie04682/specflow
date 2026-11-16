package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestHub(t *testing.T, cacheLimit int) (*Hub, string) {
	t.Helper()
	hub := newHub(cacheLimit)
	path := filepath.Join(t.TempDir(), "events.jsonl")
	return hub, path
}

func openTestLog(t *testing.T, hub *Hub, path string) {
	t.Helper()
	created, err := hub.Open("run", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !created {
		t.Fatal("expected a new log")
	}
}

func writeJournal(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

func journalLine(t *testing.T, seq uint64) string {
	t.Helper()
	event := Event{Seq: seq, At: time.Unix(int64(seq), 0).UTC(), Type: "node.event", RunID: "run"}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestHubOpenLoadsBoundedCache(t *testing.T) {
	hub, path := newTestHub(t, 2)
	openTestLog(t, hub, path)
	for i := 0; i < 5; i++ {
		if _, err := hub.Emit("run", "node.event", "a", map[string]any{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	hub.Close("run")

	reopened := newHub(2)
	if _, err := reopened.Open("run", path); err != nil {
		t.Fatal(err)
	}
	defer reopened.Close("run")

	entry := reopened.log("run")
	entry.mu.Lock()
	seq := entry.seq
	cacheStart := entry.cacheStart
	cache := entry.cacheSnapshot()
	entry.mu.Unlock()

	if seq != 5 || cacheStart != 4 {
		t.Fatalf("seq = %d, cacheStart = %d", seq, cacheStart)
	}
	if len(cache) != 2 || cache[0].Seq != 4 || cache[1].Seq != 5 {
		t.Fatalf("cache = %+v", cache)
	}
	events, err := reopened.Events("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("events = %d", len(events))
	}
}

func TestHubRepairsPartialTrailingRecord(t *testing.T) {
	hub, path := newTestHub(t, 10)
	writeJournal(t, path, journalLine(t, 1), journalLine(t, 2), `{"seq":3`)

	if _, err := hub.Open("run", path); err != nil {
		t.Fatalf("open repaired log: %v", err)
	}
	defer hub.Close("run")

	if got := hub.LastSeq("run"); got != 2 {
		t.Fatalf("last seq = %d", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `{"seq":3`) {
		t.Fatalf("partial record survived: %q", data)
	}
	if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
		t.Fatal(err)
	}
	if got := hub.LastSeq("run"); got != 3 {
		t.Fatalf("last seq after append = %d", got)
	}
}

func TestHubConcurrentOpenRepairsOnce(t *testing.T) {
	hub, path := newTestHub(t, 10)
	writeJournal(t, path, journalLine(t, 1))

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := hub.Open("run", path)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("open: %v", err)
		}
	}
	defer hub.Close("run")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "\n"); got != 1 {
		t.Fatalf("journal newlines = %d: %q", got, data)
	}
	events, err := hub.Events("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
}

func TestHubRepairsMissingTrailingNewline(t *testing.T) {
	hub, path := newTestHub(t, 10)
	writeJournal(t, path, journalLine(t, 1))

	if _, err := hub.Open("run", path); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer hub.Close("run")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("journal was not terminated: %q", data)
	}
	if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
		t.Fatal(err)
	}
	if got := hub.LastSeq("run"); got != 2 {
		t.Fatalf("last seq = %d", got)
	}
}

func TestHubRejectsMalformedMiddleRecord(t *testing.T) {
	hub, path := newTestHub(t, 10)
	writeJournal(t, path, journalLine(t, 1), "not json", journalLine(t, 2))

	_, err := hub.Open("run", path)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error = %v", err)
	}
}

func TestHubRejectsOversizedJournalRecord(t *testing.T) {
	hub, path := newTestHub(t, 10)
	line := strings.Repeat("x", eventSizeLimit+1)
	writeJournal(t, path, line)

	_, err := hub.Open("run", path)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestHubEmitRejectsOversizedAndUnserializableEvents(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)
	defer hub.Close("run")

	if _, err := hub.Emit("run", "node.event", "a", map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("expected an unserializable event error")
	}
	if got := hub.LastSeq("run"); got != 0 {
		t.Fatalf("sequence consumed on failure: %d", got)
	}

	large := strings.Repeat("x", eventSizeLimit)
	if _, err := hub.Emit("run", "node.event", "a", map[string]any{"text": large}); err == nil {
		t.Fatal("expected an oversized event error")
	}
	if got := hub.LastSeq("run"); got != 0 {
		t.Fatalf("sequence consumed on oversized event: %d", got)
	}

	event, err := hub.Emit("run", "node.event", "a", map[string]any{"text": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if event.Seq != 1 {
		t.Fatalf("seq = %d", event.Seq)
	}
}

func TestHubAppendFailureDoesNotPublish(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)
	entry := hub.log("run")
	entry.mu.Lock()
	_ = entry.file.Close()
	entry.mu.Unlock()

	if _, err := hub.Emit("run", "node.event", "a", nil); err == nil {
		t.Fatal("expected an append error")
	}
	if got := hub.LastSeq("run"); got != 0 {
		t.Fatalf("sequence advanced on append failure: %d", got)
	}
}

func TestHubWarnsOnceAtJournalThreshold(t *testing.T) {
	hub := newHub(10)
	hub.journalWarnAt = 1
	path := filepath.Join(t.TempDir(), "events.jsonl")
	openTestLog(t, hub, path)
	defer hub.Close("run")

	if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
		t.Fatal(err)
	}
	if !hub.log("run").warned {
		t.Fatal("journal warning was not recorded")
	}
}

func TestHubSubscribeReplayThenLive(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)
	defer hub.Close("run")

	for i := 0; i < 3; i++ {
		if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
			t.Fatal(err)
		}
	}

	replay, channel, sealed, cancel, err := hub.Subscribe("run", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if sealed {
		t.Fatal("live subscription was sealed")
	}
	if len(replay) != 2 || replay[0].Seq != 2 || replay[1].Seq != 3 {
		t.Fatalf("replay = %+v", replay)
	}

	if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-channel:
		if event.Seq != 4 {
			t.Fatalf("live event seq = %d", event.Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event")
	}

	cancel()
	if _, open := <-channel; open {
		t.Fatal("channel remained open after cancel")
	}
}

func TestHubCancelRemovesSubscriber(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)
	defer hub.Close("run")

	_, _, _, cancel, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	entry := hub.log("run")
	entry.mu.Lock()
	before := len(entry.subs)
	entry.mu.Unlock()
	if before != 1 {
		t.Fatalf("subscribers = %d", before)
	}
	cancel()
	cancel()
	entry.mu.Lock()
	after := len(entry.subs)
	entry.mu.Unlock()
	if after != 0 {
		t.Fatalf("subscribers after cancel = %d", after)
	}
}

func TestHubFanOut(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)
	defer hub.Close("run")

	_, first, _, cancelFirst, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelFirst()
	_, second, _, cancelSecond, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelSecond()

	if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
		t.Fatal(err)
	}
	for name, channel := range map[string]<-chan Event{"first": first, "second": second} {
		select {
		case event := <-channel:
			if event.Seq != 1 {
				t.Fatalf("%s seq = %d", name, event.Seq)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s did not receive the event", name)
		}
	}
}

func TestHubDropsSlowSubscriber(t *testing.T) {
	hub, path := newTestHub(t, 1000)
	openTestLog(t, hub, path)
	defer hub.Close("run")

	_, slow, _, cancelSlow, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelSlow()
	_, fast, _, cancelFast, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelFast()

	received := 0
	for i := 0; i < subscriberBuffer+10; i++ {
		if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
			t.Fatal(err)
		}
		select {
		case <-fast:
			received++
		default:
		}
	}
	for {
		select {
		case <-fast:
			received++
			continue
		default:
		}
		break
	}

	if received != subscriberBuffer+10 {
		t.Fatalf("fast subscriber received %d events", received)
	}
	if got := len(slow); got != subscriberBuffer {
		t.Fatalf("slow subscriber buffered %d events", got)
	}
	for i := 0; i < subscriberBuffer; i++ {
		<-slow
	}
	select {
	case _, open := <-slow:
		if open {
			t.Fatal("slow subscriber was not dropped")
		}
	default:
		t.Fatal("slow subscriber channel was not closed")
	}
}

func TestHubSealEndsStreamsAndKeepsReplay(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)

	if _, err := hub.Emit("run", "run.started", "", nil); err != nil {
		t.Fatal(err)
	}
	_, channel, _, cancel, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	if _, err := hub.Emit("run", "run.completed", "", map[string]any{"summary": "done"}); err != nil {
		t.Fatal(err)
	}
	hub.Seal("run")

	select {
	case event := <-channel:
		if event.Type != "run.completed" {
			t.Fatalf("event = %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal event was not delivered")
	}
	if _, open := <-channel; open {
		t.Fatal("channel remained open after seal")
	}

	replay, live, sealed, cancelSealed, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelSealed()
	if !sealed || live != nil {
		t.Fatalf("sealed = %v, live = %v", sealed, live)
	}
	if len(replay) != 2 {
		t.Fatalf("replay = %+v", replay)
	}
	if _, err := hub.Emit("run", "node.event", "a", nil); err == nil {
		t.Fatal("emit after seal succeeded")
	}
}

func TestHubBeginUnsealsRetainedLog(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)
	if _, err := hub.Emit("run", "run.failed", "", nil); err != nil {
		t.Fatal(err)
	}
	hub.Seal("run")

	replay, _, sealed, cancel, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if !sealed || len(replay) != 1 {
		t.Fatalf("sealed = %v, replay = %d", sealed, len(replay))
	}

	hub.Begin("run")
	event, err := hub.Emit("run", "run.started", "", nil)
	if err != nil {
		t.Fatalf("emit after begin: %v", err)
	}
	if event.Seq != 2 {
		t.Fatalf("seq = %d", event.Seq)
	}
}

func TestHubSealedReplayReadersKeepLogOpen(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)
	if _, err := hub.Emit("run", "run.completed", "", nil); err != nil {
		t.Fatal(err)
	}
	hub.Seal("run")

	replayOne, _, _, cancelOne, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	replayTwo, _, _, cancelTwo, err := hub.Subscribe("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayOne) != 1 || len(replayTwo) != 1 {
		t.Fatalf("replays = %d, %d", len(replayOne), len(replayTwo))
	}

	cancelOne()
	cancelOne()
	if hub.log("run") == nil {
		t.Fatal("releasing one replay reader closed the log")
	}
	cancelTwo()
	if hub.log("run") != nil {
		t.Fatal("releasing the last replay reader left the log open")
	}
}

func TestHubCloseReopensJournal(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)

	if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
		t.Fatal(err)
	}
	hub.Close("run")

	reopened, err := hub.Open("run", path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened {
		t.Fatal("expected a fresh log after close")
	}
	defer hub.Close("run")

	events, err := hub.Events("run", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	if _, err := hub.Emit("run", "node.event", "a", nil); err != nil {
		t.Fatal(err)
	}
	if got := hub.LastSeq("run"); got != 2 {
		t.Fatalf("last seq = %d", got)
	}
}

func TestHubConcurrentLifecycle(t *testing.T) {
	hub, path := newTestHub(t, 16)
	openTestLog(t, hub, path)
	defer hub.Close("run")

	var wg sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = hub.Emit("run", "node.event", "a", map[string]any{"i": i})
			}
		}()
	}
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_, _, _, cancel, err := hub.Subscribe("run", 0)
				if err == nil {
					cancel()
				}
			}
		}()
	}
	wg.Wait()

	hub.Seal("run")
	if got := hub.LastSeq("run"); got == 0 {
		t.Fatal("no events were committed")
	}
}

func TestHubAppendFailureIsReported(t *testing.T) {
	hub, path := newTestHub(t, 10)
	openTestLog(t, hub, path)
	entry := hub.log("run")
	entry.mu.Lock()
	_ = entry.file.Close()
	entry.mu.Unlock()

	_, err := hub.Emit("run", "node.event", "a", nil)
	if err == nil || !strings.Contains(err.Error(), "append") {
		t.Fatalf("error = %v", err)
	}
}

func TestHubEmitUnknownRun(t *testing.T) {
	hub := newHub(10)
	if _, err := hub.Emit("missing", "node.event", "", nil); err == nil {
		t.Fatal("expected an error for a missing log")
	}
	if _, err := hub.Events("missing", 0); err != nil {
		t.Fatalf("events for a missing log: %v", err)
	}
}
