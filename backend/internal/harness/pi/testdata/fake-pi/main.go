// Command fake-pi is a scripted stand-in for Pi's RPC mode.
//
// It exists so the adapter's lifecycle, framing, question, injection, and
// shutdown paths can be tested without Pi installed, without credentials, and
// without network access. It speaks the documented records; it is not a Pi
// emulator.
//
// Environment:
//
//	FAKE_PI_VERSION          version printed for --version (default 1.2.3)
//	FAKE_PI_SCRIPT           JSONL file of records replayed after a prompt
//	FAKE_PI_CAPTURE          file that receives every command as JSONL
//	FAKE_PI_BRIDGE_FAIL=1    fail at startup as an incompatible extension would
//	FAKE_PI_NO_MARKER=1      do not emit the bridge-ready notification
//	FAKE_PI_IGNORE_GET_STATE=1  never answer get_state (startup timeout test)
//	FAKE_PI_HANDLED_PROMPT=1    answer prompt with disposition "handled"
//	FAKE_PI_PROMPT_ERROR     answer prompt with success:false and this error
//	FAKE_PI_EXIT_AT_START    exit with this status before answering anything
//	FAKE_PI_NO_GET_COMMANDS=1   reply that get_commands is unsupported
//	FAKE_PI_OMIT_SKILL=1        report no skill commands even when --skill is set
//	FAKE_PI_BAD_RECORD=1        emit one malformed line after the prompt
//	FAKE_PI_EXIT_BEFORE_SETTLE=1 exit 1 after the script without settling
//	FAKE_PI_IGNORE_STDIN_CLOSE=1 stay alive after stdin closes (escalation test)
//	FAKE_PI_STDERR           text written to stderr at startup
//	FAKE_PI_PIDFILE          file that receives this process's pid at startup
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	bridgeMarker = "specflow-bridge:ready"
	dialogWait   = 10 * time.Second
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("pi %s\n", version())
			return
		}
	}

	if os.Getenv("FAKE_PI_STDERR") != "" {
		fmt.Fprintln(os.Stderr, os.Getenv("FAKE_PI_STDERR"))
	}

	if value := os.Getenv("FAKE_PI_EXIT_AT_START"); value != "" {
		code, err := strconv.Atoi(value)
		if err != nil || code == 0 {
			code = 1
		}
		os.Exit(code)
	}

	if path := os.Getenv("FAKE_PI_PIDFILE"); path != "" {
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "fake-pi: pidfile: %v\n", err)
		}
	}

	if os.Getenv("FAKE_PI_IGNORE_STDIN_CLOSE") == "1" {
		// Escalation test: neither an orderly shutdown nor SIGTERM stops it.
		signal.Ignore(syscall.SIGTERM)
	}

	if os.Getenv("FAKE_PI_BRIDGE_FAIL") == "1" {
		fmt.Fprintln(os.Stderr, "failed to load extension specflow-bridge.ts: unsupported extension API")
		os.Exit(2)
	}

	s := &server{
		dialogs:    make(chan map[string]any, 4),
		injections: make(chan struct{}, 4),
		script:     loadScript(os.Getenv("FAKE_PI_SCRIPT")),
	}

	if s.capturePath = os.Getenv("FAKE_PI_CAPTURE"); s.capturePath != "" {
		file, err := os.OpenFile(s.capturePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fake-pi: capture: %v\n", err)
			os.Exit(1)
		}
		s.capture = file
	}

	if hasArg("-e") && os.Getenv("FAKE_PI_NO_MARKER") != "1" {
		s.emit(map[string]any{
			"type":       "extension_ui_request",
			"id":         "bridge-marker",
			"method":     "notify",
			"message":    bridgeMarker,
			"notifyType": "info",
		})
	}

	s.serve()
}

func version() string {
	if value := os.Getenv("FAKE_PI_VERSION"); value != "" {
		return value
	}
	return "1.2.3"
}

func hasArg(flag string) bool {
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
	}
	return false
}

// skillPaths returns every path passed with --skill, which is how the adapter
// binds a node's skill.
func skillPaths(args []string) []string {
	var paths []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--skill" && i+1 < len(args) {
			paths = append(paths, args[i+1])
			i++
		}
	}
	return paths
}

type server struct {
	dialogs    chan map[string]any
	injections chan struct{}
	script     []map[string]any

	capturePath string
	capture     *os.File

	writeMu sync.Mutex
}

func (s *server) serve() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var command map[string]any
		if err := json.Unmarshal(line, &command); err != nil {
			s.emit(map[string]any{
				"type":    "response",
				"command": "parse",
				"success": false,
				"error":   "failed to parse command: " + err.Error(),
			})
			continue
		}

		s.captureCommand(command)
		s.handle(command)
	}

	if os.Getenv("FAKE_PI_IGNORE_STDIN_CLOSE") == "1" {
		// A harness that ignores the orderly shutdown request; the adapter
		// must escalate rather than wait forever.
		time.Sleep(time.Hour)
	}
}

func (s *server) handle(command map[string]any) {
	switch command["type"] {
	case "get_state":
		if os.Getenv("FAKE_PI_IGNORE_GET_STATE") == "1" {
			return
		}
		s.respond(command, map[string]any{"sessionId": "fake-session", "model": "fake/model"})

	case "get_commands":
		if os.Getenv("FAKE_PI_NO_GET_COMMANDS") == "1" {
			s.respondError(command, "unknown command: get_commands")
			return
		}
		commands := []map[string]any{}
		if os.Getenv("FAKE_PI_OMIT_SKILL") != "1" {
			for _, path := range skillPaths(os.Args[1:]) {
				commands = append(commands, map[string]any{
					"name":   "skill:" + filepath.Base(path),
					"source": "skill",
				})
			}
		}
		s.respond(command, map[string]any{"commands": commands})

	case "prompt":
		if message := os.Getenv("FAKE_PI_PROMPT_ERROR"); message != "" {
			s.respondError(command, message)
			return
		}
		if os.Getenv("FAKE_PI_HANDLED_PROMPT") == "1" {
			s.respondDisposition(command, "handled")
			return
		}
		s.respondDisposition(command, "started")
		if os.Getenv("FAKE_PI_BAD_RECORD") == "1" {
			s.writeLine("this is not json")
			return
		}
		go s.replay()

	case "steer", "follow_up":
		s.respondDisposition(command, "queued")
		select {
		case s.injections <- struct{}{}:
		default:
		}

	case "abort":
		s.respond(command, nil)
		s.emit(map[string]any{"type": "agent_settled", "aborted": true})

	case "extension_ui_response":
		select {
		case s.dialogs <- command:
		default:
		}

	default:
		s.respond(command, nil)
	}
}

// replay emits the script in order. Dialog requests wait for the client's
// answer, which is the round trip the adapter is responsible for.
func (s *server) replay() {
	for _, rec := range s.script {
		if ms, ok := rec["__sleep_ms"].(float64); ok {
			time.Sleep(time.Duration(ms) * time.Millisecond)
			continue
		}
		if rec["__wait_injection"] == true {
			select {
			case <-s.injections:
			case <-time.After(dialogWait):
			}
			continue
		}

		s.emit(rec)

		if rec["type"] == "extension_ui_request" {
			select {
			case response := <-s.dialogs:
				s.captureCommand(response)
			case <-time.After(dialogWait):
				s.emit(map[string]any{"type": "agent_settled", "aborted": true})
				return
			}
		}
	}

	if os.Getenv("FAKE_PI_EXIT_BEFORE_SETTLE") == "1" {
		os.Exit(1)
	}
}

func (s *server) respond(command map[string]any, data map[string]any) {
	response := map[string]any{
		"id":      command["id"],
		"type":    "response",
		"command": command["type"],
		"success": true,
	}
	if data != nil {
		response["data"] = data
	}
	s.emit(response)
}

func (s *server) respondDisposition(command map[string]any, disposition string) {
	s.respond(command, map[string]any{"disposition": disposition})
}

// respondError rejects a command the way Pi rejects an unacceptable prompt.
func (s *server) respondError(command map[string]any, message string) {
	s.emit(map[string]any{
		"id":      command["id"],
		"type":    "response",
		"command": command["type"],
		"success": false,
		"error":   message,
	})
}

func (s *server) emit(rec map[string]any) {
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	s.writeLine(string(line))
}

func (s *server) writeLine(line string) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if _, err := io.WriteString(os.Stdout, line+"\n"); err != nil {
		os.Exit(1)
	}
}

func (s *server) captureCommand(command map[string]any) {
	if s.capture == nil {
		return
	}

	line, err := json.Marshal(command)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.capture.Write(append(line, '\n'))
}

func loadScript(path string) []map[string]any {
	if path == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake-pi: open script: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var script []map[string]any
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			fmt.Fprintf(os.Stderr, "fake-pi: parse script line %q: %v\n", line, err)
			os.Exit(1)
		}
		script = append(script, rec)
	}
	return script
}
