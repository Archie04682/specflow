package orchestrator

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event is one record in a run's live stream. Seq is the run's own counter:
// the harness numbers events per execution, which is not usable across nodes.
type Event struct {
	Seq    uint64         `json:"seq"`
	At     time.Time      `json:"at"`
	Type   string         `json:"type"`
	RunID  string         `json:"runId"`
	NodeID string         `json:"nodeId,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
}

const (
	defaultReplayCache      = 4096
	subscriberBuffer        = 256
	eventSizeLimit          = 4 << 20
	defaultJournalWarnBytes = int64(256 << 20)
)

// Stream is one client's view of a run's event stream. Replay is delivered
// before Events, so the caller writes Replay first and then drains Events.
// A sealed stream has no live channel and can be served entirely from Replay.
type Stream struct {
	Replay  []Event
	Events  <-chan Event
	Cancel  func()
	Sealed  bool
	LastSeq uint64
}

// Hub stores and fans out run events. Events are retained in a bounded cache
// and appended to a journal, which is the replay source after a restart.
type Hub struct {
	mu      sync.Mutex
	logs    map[string]*eventLog
	opening sync.Map // runID -> *sync.Mutex; serializes journal repair per run

	cacheLimit    int
	maxEventBytes int
	journalWarnAt int64
}

// NewHub returns an empty event hub with the default cache and journal limits.
func NewHub() *Hub {
	return newHub(defaultReplayCache)
}

func newHub(cacheLimit int) *Hub {
	return &Hub{
		logs:          make(map[string]*eventLog),
		cacheLimit:    cacheLimit,
		maxEventBytes: eventSizeLimit,
		journalWarnAt: defaultJournalWarnBytes,
	}
}

type eventLog struct {
	mu sync.Mutex

	runID string
	path  string

	seq        uint64
	cache      []Event
	cacheHead  int
	cacheStart uint64

	file     *os.File
	fileSize int64

	sealed bool
	closed bool
	subs   map[chan Event]struct{}
	warned bool

	readers int
	onIdle  func()

	cacheLimit    int
	maxEventBytes int
	warnAt        int64
}

// Open prepares a run's event log, loading its journal so replay works after a
// restart. An empty path keeps the log in memory only. It reports whether the
// log was created, which lets a caller know it opened a terminal run on demand.
func (h *Hub) Open(runID, path string) (bool, error) {
	// Serialize opens for the same run so two repairs cannot append two trailing
	// newlines. Different runs still open concurrently.
	openLock, _ := h.opening.LoadOrStore(runID, &sync.Mutex{})
	mutex := openLock.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()

	h.mu.Lock()
	if existing := h.logs[runID]; existing != nil {
		existing.mu.Lock()
		closed := existing.closed
		existing.mu.Unlock()
		if !closed {
			h.mu.Unlock()
			return false, nil
		}
		delete(h.logs, runID)
	}
	h.mu.Unlock()

	// Read the journal outside the hub lock: a large terminal replay must not
	// block event publication for every other run.
	entry, err := h.openLog(runID, path)
	if err != nil {
		return false, err
	}

	h.mu.Lock()
	if existing := h.logs[runID]; existing != nil {
		h.mu.Unlock()
		entry.closeUnused()
		return false, nil
	}
	h.logs[runID] = entry
	h.mu.Unlock()
	return true, nil
}

func (h *Hub) openLog(runID, path string) (*eventLog, error) {
	entry := &eventLog{
		runID:         runID,
		path:          path,
		subs:          make(map[chan Event]struct{}),
		cacheLimit:    h.cacheLimit,
		maxEventBytes: h.maxEventBytes,
		warnAt:        h.journalWarnAt,
	}
	entry.onIdle = func() { h.removeLog(runID, entry) }

	if path == "" {
		entry.cacheStart = 1
		return entry, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create event journal directory: %w", err)
	}

	cache, cacheStart, seq, size, err := readJournal(path, h.cacheLimit, h.maxEventBytes)
	if err != nil {
		return nil, err
	}
	entry.cache = cache
	entry.cacheStart = cacheStart
	entry.seq = seq
	entry.fileSize = size

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open event journal %s: %w", path, err)
	}
	entry.file = file
	return entry, nil
}

// Emit stamps a new event, persists it, and publishes it. A subscriber that
// cannot keep up is dropped rather than blocking the run; it reconnects with
// its last sequence number and catches up from the journal.
func (h *Hub) Emit(runID, eventType, nodeID string, data map[string]any) (Event, error) {
	entry := h.log(runID)
	if entry == nil {
		return Event{}, fmt.Errorf("run %s has no event log", runID)
	}
	return entry.emit(eventType, nodeID, data, h.maxEventBytes)
}

func (l *eventLog) emit(eventType, nodeID string, data map[string]any, maxBytes int) (Event, error) {
	event, dropped, warn, err := l.emitLocked(eventType, nodeID, data, maxBytes)
	if dropped > 0 {
		log.Printf("specflow: run %s: dropped %d slow event subscriber(s)", l.runID, dropped)
	}
	if warn {
		log.Printf("specflow: run %s: event journal reached %d bytes; retention is not truncated", l.runID, l.fileSizeAfter())
	}
	return event, err
}

func (l *eventLog) emitLocked(eventType, nodeID string, data map[string]any, maxBytes int) (Event, int, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return Event{}, 0, false, errors.New("event log is closed")
	}
	if l.sealed {
		return Event{}, 0, false, errors.New("event log is sealed")
	}

	event := Event{
		Seq:    l.seq + 1,
		At:     time.Now(),
		Type:   eventType,
		RunID:  l.runID,
		NodeID: nodeID,
		Data:   data,
	}
	line, err := json.Marshal(event)
	if err != nil {
		return Event{}, 0, false, fmt.Errorf("encode event: %w", err)
	}
	if len(line)+1 > maxBytes {
		return Event{}, 0, false, fmt.Errorf("event exceeds %d bytes", maxBytes)
	}

	if l.file != nil {
		previous := l.fileSize
		n, err := l.file.Write(append(line, '\n'))
		if err != nil || n != len(line)+1 {
			_ = l.file.Truncate(previous)
			if err != nil {
				return Event{}, 0, false, fmt.Errorf("append event journal: %w", err)
			}
			return Event{}, 0, false, errors.New("append event journal: short write")
		}
		l.fileSize += int64(n)
	}

	l.seq = event.Seq
	l.addCache(event)
	dropped := l.fanout(event)
	warn := l.warnIfLarge()
	return event, dropped, warn, nil
}

func (l *eventLog) fileSizeAfter() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fileSize
}

func (l *eventLog) addCache(event Event) {
	if l.cacheLimit <= 0 {
		l.cacheStart = event.Seq + 1
		return
	}
	if len(l.cache) < l.cacheLimit {
		l.cache = append(l.cache, event)
	} else {
		l.cache[l.cacheHead] = event
		l.cacheHead = (l.cacheHead + 1) % l.cacheLimit
	}
	if len(l.cache) == l.cacheLimit {
		l.cacheStart = l.cache[l.cacheHead].Seq
	} else {
		l.cacheStart = l.cache[0].Seq
	}
}

func (l *eventLog) fanout(event Event) int {
	dropped := 0
	for subscriber := range l.subs {
		select {
		case subscriber <- event:
		default:
			close(subscriber)
			delete(l.subs, subscriber)
			dropped++
		}
	}
	return dropped
}

func (l *eventLog) warnIfLarge() bool {
	if l.warnAt <= 0 || l.warned || l.fileSize < l.warnAt {
		return false
	}
	l.warned = true
	return true
}

func (l *eventLog) cacheSnapshot() []Event {
	limit := l.cacheLimit
	if limit <= 0 || len(l.cache) == 0 {
		return nil
	}
	if len(l.cache) < limit {
		return append([]Event(nil), l.cache...)
	}
	events := make([]Event, 0, limit)
	events = append(events, l.cache[l.cacheHead:]...)
	events = append(events, l.cache[:l.cacheHead]...)
	return events
}

// Subscribe returns the events after a sequence number, then live ones. Replay
// is returned separately so a late subscriber never allocates a channel as
// large as the run. The cancel function must be called when the client
// disconnects.
func (h *Hub) Subscribe(runID string, after uint64) (replay []Event, events <-chan Event, sealed bool, cancel func(), err error) {
	entry := h.log(runID)
	if entry == nil {
		return nil, nil, true, func() {}, nil
	}
	return entry.subscribe(after)
}

func (l *eventLog) subscribe(after uint64) ([]Event, <-chan Event, bool, func(), error) {
	l.mu.Lock()
	if l.closed || l.sealed {
		cache := l.cacheSnapshot()
		cacheStart := l.cacheStart
		seq := l.seq
		path := l.path
		closed := l.closed
		if !closed {
			l.readers++
		}
		l.mu.Unlock()

		replay, err := replayEvents(cache, cacheStart, seq, path, after, l.maxEventBytes)
		if err != nil {
			if !closed {
				l.release()
			}
			return nil, nil, true, func() {}, err
		}
		if closed {
			return replay, nil, true, func() {}, nil
		}
		return replay, nil, true, once(func() { l.release() }), nil
	}

	channel := make(chan Event, subscriberBuffer)
	l.subs[channel] = struct{}{}
	cache := l.cacheSnapshot()
	cacheStart := l.cacheStart
	seq := l.seq
	path := l.path
	l.mu.Unlock()

	replay, err := replayEvents(cache, cacheStart, seq, path, after, l.maxEventBytes)
	if err != nil {
		l.unsubscribe(channel)
		return nil, nil, false, nil, err
	}
	return replay, channel, false, once(func() { l.unsubscribe(channel) }), nil
}

func once(fn func()) func() {
	var run sync.Once
	return func() { run.Do(fn) }
}

func (l *eventLog) unsubscribe(channel chan Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.subs[channel]; exists {
		delete(l.subs, channel)
		close(channel)
	}
}

func (l *eventLog) release() {
	l.mu.Lock()
	if l.readers > 0 {
		l.readers--
	}
	closeNow := l.readers == 0 && (l.sealed || l.closed) && !l.closed
	if closeNow {
		l.closed = true
		if l.file != nil {
			_ = l.file.Close()
			l.file = nil
		}
	}
	l.mu.Unlock()
	if closeNow && l.onIdle != nil {
		l.onIdle()
	}
}

// Events returns the retained events after a sequence number. Unlike
// Subscribe it does not create a live subscriber; it is used for one-shot
// replay and tests.
func (h *Hub) Events(runID string, after uint64) ([]Event, error) {
	entry := h.log(runID)
	if entry == nil {
		return nil, nil
	}
	return entry.events(after)
}

func (l *eventLog) events(after uint64) ([]Event, error) {
	l.mu.Lock()
	cache := l.cacheSnapshot()
	cacheStart := l.cacheStart
	seq := l.seq
	path := l.path
	l.mu.Unlock()

	return replayEvents(cache, cacheStart, seq, path, after, l.maxEventBytes)
}

// LastSeq returns the last retained sequence number, or zero when no log
// exists.
func (h *Hub) LastSeq(runID string) uint64 {
	entry := h.log(runID)
	if entry == nil {
		return 0
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.seq
}

// Seal closes every subscriber channel and marks the log terminal. The journal
// and cache stay readable so a late subscriber can replay the terminal event.
func (h *Hub) Seal(runID string) {
	entry := h.log(runID)
	if entry == nil {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.sealed {
		return
	}
	entry.sealed = true
	for subscriber := range entry.subs {
		close(subscriber)
		delete(entry.subs, subscriber)
	}
}

// closeUnused releases a log that was opened concurrently but lost the
// installation race in Open. It has no subscribers.
func (l *eventLog) closeUnused() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// Close seals the log, closes its journal, and removes it from the hub. The
// journal file stays on disk; a later Open recreates the log from it.
func (h *Hub) Close(runID string) {
	h.mu.Lock()
	entry := h.logs[runID]
	delete(h.logs, runID)
	h.mu.Unlock()

	if entry == nil {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.sealed = true
	entry.closed = true
	for subscriber := range entry.subs {
		close(subscriber)
		delete(entry.subs, subscriber)
	}
	if entry.file != nil {
		_ = entry.file.Close()
		entry.file = nil
	}
}

// Begin starts a new execution epoch on a retained log. A resume or retry can
// find the previous epoch sealed; it must accept new events.
func (h *Hub) Begin(runID string) {
	entry := h.log(runID)
	if entry == nil {
		return
	}
	entry.mu.Lock()
	entry.sealed = false
	entry.mu.Unlock()
}

// CloseIfIdle releases a log that has no subscribers. It is used when a
// client disconnects from a run that has not started, so a pending run does
// not keep a journal open.
func (h *Hub) CloseIfIdle(runID string) {
	h.mu.Lock()
	entry := h.logs[runID]
	h.mu.Unlock()
	if entry == nil {
		return
	}

	entry.mu.Lock()
	if entry.closed || entry.sealed || entry.readers != 0 || len(entry.subs) != 0 {
		entry.mu.Unlock()
		return
	}
	entry.closed = true
	if entry.file != nil {
		_ = entry.file.Close()
		entry.file = nil
	}
	entry.mu.Unlock()

	h.mu.Lock()
	if h.logs[runID] == entry {
		delete(h.logs, runID)
	}
	h.mu.Unlock()
}

func (h *Hub) removeLog(runID string, entry *eventLog) {
	h.mu.Lock()
	if h.logs[runID] == entry {
		delete(h.logs, runID)
	}
	h.mu.Unlock()
}

// CloseAll closes every retained log. It is used on engine shutdown so a log
// held only by a sealed replay reader is not lost to process exit.
func (h *Hub) CloseAll() {
	h.mu.Lock()
	ids := make([]string, 0, len(h.logs))
	for runID := range h.logs {
		ids = append(ids, runID)
	}
	h.mu.Unlock()
	for _, runID := range ids {
		h.Close(runID)
	}
}

func (h *Hub) log(runID string) *eventLog {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.logs[runID]
}

func replayEvents(cache []Event, cacheStart, seq uint64, path string, after uint64, maxBytes int) ([]Event, error) {
	if after >= seq {
		return nil, nil
	}

	var replay []Event
	if after+1 < cacheStart {
		prefix, err := readJournalAfter(path, after, cacheStart, maxBytes)
		if err != nil {
			return nil, err
		}
		replay = append(replay, prefix...)
	}
	for _, event := range cache {
		if event.Seq > after {
			replay = append(replay, event)
		}
	}
	return replay, nil
}

// readJournal reads a run's append-only journal, repairs a partial trailing
// record, keeps the last cacheLimit events in memory, and returns the last
// sequence number and the repaired file size.
func readJournal(path string, cacheLimit, maxBytes int) ([]Event, uint64, uint64, int64, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 1, 0, 0, nil
	}
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("open event journal %s: %w", path, err)
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64*1024)
	var lineStart int64
	var lineNumber int
	var seq uint64
	ring := make([]Event, 0, max(0, cacheLimit))
	head := 0

	add := func(event Event) {
		if cacheLimit <= 0 {
			return
		}
		if len(ring) < cacheLimit {
			ring = append(ring, event)
			return
		}
		ring[head] = event
		head = (head + 1) % cacheLimit
	}

	for {
		line, readErr := readJournalLine(reader, maxBytes)
		if readErr == errLineTooLong {
			return nil, 0, 0, 0, fmt.Errorf("event journal %s line %d exceeds %d bytes", path, lineNumber+1, maxBytes)
		}
		if readErr != nil && readErr != io.EOF {
			return nil, 0, 0, 0, fmt.Errorf("read event journal %s: %w", path, readErr)
		}
		if len(line) == 0 {
			if readErr == io.EOF {
				break
			}
			return nil, 0, 0, 0, fmt.Errorf("event journal %s line %d is empty", path, lineNumber+1)
		}

		terminated := line[len(line)-1] == '\n'
		content := line
		if terminated {
			content = line[:len(line)-1]
		}
		if len(content) == 0 {
			return nil, 0, 0, 0, fmt.Errorf("event journal %s line %d is empty", path, lineNumber+1)
		}
		lineNumber++

		var event Event
		if err := json.Unmarshal(content, &event); err != nil {
			if !terminated {
				// A crash can leave a partial last record. Drop it rather than
				// making the whole run unreplayable.
				if err := file.Truncate(lineStart); err != nil {
					return nil, 0, 0, 0, fmt.Errorf("repair event journal %s: %w", path, err)
				}
				return orderedCache(ring, head, cacheLimit), cacheStart(ring, head, cacheLimit, seq), seq, lineStart, nil
			}
			return nil, 0, 0, 0, fmt.Errorf("parse event journal %s line %d: %w", path, lineNumber, err)
		}
		if err := validateEvent(event, seq, lineNumber); err != nil {
			return nil, 0, 0, 0, err
		}
		seq = event.Seq
		add(event)

		if !terminated {
			// A complete final record without a newline is valid; terminate it
			// so the next append starts on its own line.
			if _, err := file.Seek(0, io.SeekEnd); err != nil {
				return nil, 0, 0, 0, fmt.Errorf("repair event journal %s: %w", path, err)
			}
			if _, err := file.Write([]byte{'\n'}); err != nil {
				return nil, 0, 0, 0, fmt.Errorf("repair event journal %s: %w", path, err)
			}
			size := lineStart + int64(len(line)) + 1
			return orderedCache(ring, head, cacheLimit), cacheStart(ring, head, cacheLimit, seq), seq, size, nil
		}

		lineStart += int64(len(line))
		if readErr == io.EOF {
			break
		}
	}

	return orderedCache(ring, head, cacheLimit), cacheStart(ring, head, cacheLimit, seq), seq, lineStart, nil
}

var errLineTooLong = errors.New("event record exceeds limit")

// readJournalLine returns one record including its newline. A final record
// without a newline is returned with io.EOF. The slice is a copy, so the
// reader can be reused immediately.
func readJournalLine(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxBytes {
			return nil, errLineTooLong
		}
		line = append(line, fragment...)
		switch err {
		case nil:
			return line, nil
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			return line, io.EOF
		default:
			return line, err
		}
	}
}

func validateEvent(event Event, previous uint64, line int) error {
	if event.Seq == 0 || event.Seq <= previous {
		return fmt.Errorf("event sequence %d is not increasing at line %d", event.Seq, line)
	}
	return nil
}

func orderedCache(ring []Event, head, limit int) []Event {
	if limit <= 0 || len(ring) == 0 {
		return nil
	}
	if len(ring) < limit {
		return ring
	}
	events := make([]Event, 0, limit)
	events = append(events, ring[head:]...)
	events = append(events, ring[:head]...)
	return events
}

func cacheStart(ring []Event, head, limit int, seq uint64) uint64 {
	events := orderedCache(ring, head, limit)
	if len(events) == 0 {
		return seq + 1
	}
	return events[0].Seq
}

// readJournalAfter reads the journal range (after, before). A partial trailing
// record is ignored; Open repairs it before any replay.
func readJournalAfter(path string, after, before uint64, maxBytes int) ([]Event, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open event journal %s: %w", path, err)
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64*1024)
	var events []Event
	for {
		line, readErr := readJournalLine(reader, maxBytes)
		if readErr == errLineTooLong {
			return nil, fmt.Errorf("event journal %s contains a record over %d bytes", path, maxBytes)
		}
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("read event journal %s: %w", path, readErr)
		}
		if len(line) == 0 {
			return events, nil
		}
		if line[len(line)-1] != '\n' {
			// A trailing partial record is ignored; Open repairs it before replay.
			return events, nil
		}
		content := line[:len(line)-1]
		if len(content) == 0 {
			return nil, fmt.Errorf("event journal %s contains an empty record", path)
		}
		var event Event
		if err := json.Unmarshal(content, &event); err != nil {
			return nil, fmt.Errorf("parse event journal %s: %w", path, err)
		}
		if event.Seq > after {
			if before > 0 && event.Seq >= before {
				return events, nil
			}
			events = append(events, event)
		}
		if readErr == io.EOF {
			return events, nil
		}
	}
}
