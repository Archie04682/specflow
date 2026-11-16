package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"example.com/specflow/backend/internal/api"
	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/pi"
	"example.com/specflow/backend/internal/orchestrator"
)

// testClient disables keep-alives so connection goroutines do not outlive a
// test and get mistaken for an execution leak.
var testClient = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

// realPiSkipReason is pure so the gate can be tested without Pi installed.
func realPiSkipReason(getenv func(string) string, lookPath func(string) (string, error)) string {
	if getenv("PI_E2E") != "1" {
		return "set PI_E2E=1 to run tests against a real Pi"
	}
	if strings.TrimSpace(getenv("PI_E2E_MODEL")) == "" {
		return "set PI_E2E_MODEL to a cheap model pattern (for example provider/model)"
	}
	if _, err := lookPath("pi"); err != nil {
		return "pi is not on PATH"
	}
	return ""
}

// requireE2E opens the opt-in gate for scenarios that do not need a model.
func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("PI_E2E") != "1" {
		t.Skip("set PI_E2E=1 to run tests against a real Pi")
	}
}

// requireRealPiBinary skips unless the opt-in gate is open and logs the exact
// Pi version, so a failure names the runtime it ran against.
func requireRealPiBinary(t *testing.T) string {
	t.Helper()

	requireE2E(t)
	binary, err := exec.LookPath("pi")
	if err != nil {
		t.Skipf("pi is not on PATH: %v", err)
	}
	version, err := exec.Command(binary, "--version").Output()
	if err != nil {
		t.Fatalf("pi --version: %v", err)
	}
	t.Logf("running against %s (%s)", binary, strings.TrimSpace(string(version)))
	return binary
}

// requireRealPi adds the model requirement for scenarios that make model calls.
func requireRealPi(t *testing.T) (model, binary string) {
	t.Helper()

	binary = requireRealPiBinary(t)
	model = strings.TrimSpace(os.Getenv("PI_E2E_MODEL"))
	if model == "" {
		t.Skip("set PI_E2E_MODEL to a cheap model pattern (for example provider/model)")
	}
	t.Logf("using model %s", model)
	return model, binary
}

func newPiAdapter(t *testing.T, binary string) *pi.Adapter {
	t.Helper()

	return pi.New(pi.Options{
		Binary:         binary,
		CacheDir:       t.TempDir(),
		StartupTimeout: 30 * time.Second,
		CommandTimeout: 90 * time.Second,
		ShutdownGrace:  5 * time.Second,
	})
}

// newServer builds the real router, engine, and registry around one adapter.
func newServer(t *testing.T, id string, adapter harness.Adapter, workflows map[string]string) (*httptest.Server, *orchestrator.Engine, string) {
	t.Helper()
	return newServerOn(t, id, adapter, newFixture(t), workflows)
}

// newServerOn is newServer with a caller-provided fixture, so the full
// workflow can add the real OpenSpec skills before the server starts.
func newServerOn(t *testing.T, id string, adapter harness.Adapter, project *fixture, workflows map[string]string) (*httptest.Server, *orchestrator.Engine, string) {
	t.Helper()

	for name, body := range workflows {
		project.writeWorkflow(t, name, body)
	}

	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter { return adapter }); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	engine := orchestrator.New(registry)
	server := httptest.NewServer(api.NewRouterWithDeps(fstest.MapFS{}, registry, engine))

	t.Cleanup(server.Close)
	t.Cleanup(engine.Close)
	return server, engine, project.Root
}

func createRun(t *testing.T, server *httptest.Server, project, workflow, objective string) orchestrator.Run {
	t.Helper()

	status, body := postJSON(t, server.URL+"/api/runs", map[string]string{
		"project":   project,
		"workflow":  workflow,
		"objective": objective,
	})
	if status != http.StatusCreated {
		t.Fatalf("create run: %d %s", status, body)
	}

	var run orchestrator.Run
	if err := json.Unmarshal(body, &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	return run
}

// waitRun polls the read endpoint until the run is terminal or the deadline
// passes. It deliberately does not use the event stream: the tests assert the
// stream separately.
func waitRun(t *testing.T, server *httptest.Server, project, runID string, timeout time.Duration) orchestrator.Run {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		run := getRun(t, server, project, runID)
		if run.Status.Terminal() {
			return run
		}
		time.Sleep(100 * time.Millisecond)
	}
	run := getRun(t, server, project, runID)
	t.Fatalf("run %s is still %s after %s (node %s)", runID, run.Status, timeout, run.CurrentNode)
	return run
}

func getRun(t *testing.T, server *httptest.Server, project, runID string) orchestrator.Run {
	t.Helper()

	response, err := testClient.Get(runURL(server.URL, project, runID, ""))
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("get run: %d %s", response.StatusCode, body)
	}

	var run orchestrator.Run
	if err := json.Unmarshal(body, &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	return run
}

func postJSON(t *testing.T, target string, body any) (int, []byte) {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	response, err := testClient.Post(target, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("post %s: %v", target, err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response.StatusCode, data
}

func runURL(base, project, runID, suffix string) string {
	query := url.Values{"project": {project}}.Encode()
	return base + "/api/runs/" + runID + suffix + "?" + query
}

// assertNoGoroutineLeak gives the engine and HTTP server a moment to release
// their goroutines, then reports a dump when the count stays elevated.
func assertNoGoroutineLeak(t *testing.T, before int) {
	t.Helper()

	const slack = 2
	deadline := time.Now().Add(3 * time.Second)
	var after int
	for time.Now().Before(deadline) {
		after = runtime.NumGoroutine()
		if after <= before+slack {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	buffer := make([]byte, 1<<20)
	buffer = buffer[:runtime.Stack(buffer, true)]
	t.Fatalf("goroutines = %d, started from %d\n%s", after, before, buffer)
}

// openFDCount returns the process's open descriptor count when the platform
// exposes it.
func openFDCount() (int, bool) {
	for _, directory := range []string{"/dev/fd", "/proc/self/fd"} {
		entries, err := os.ReadDir(directory)
		if err == nil {
			return len(entries), true
		}
	}
	return 0, false
}

func assertNoFDLeak(t *testing.T, before int) {
	t.Helper()

	after, ok := openFDCount()
	if !ok {
		t.Skip("the platform does not expose its open descriptors")
	}
	if after > before+8 {
		t.Fatalf("open descriptors = %d, started from %d", after, before)
	}
}

func TestRealPiSkipReason(t *testing.T) {
	havePi := func(string) (string, error) { return "/usr/local/bin/pi", nil }
	noPi := func(string) (string, error) { return "", fmt.Errorf("not found") }

	cases := []struct {
		name string
		env  map[string]string
		look func(string) (string, error)
		want string
	}{
		{"opt-in missing", map[string]string{}, havePi, "PI_E2E"},
		{"model missing", map[string]string{"PI_E2E": "1"}, havePi, "PI_E2E_MODEL"},
		{"pi missing", map[string]string{"PI_E2E": "1", "PI_E2E_MODEL": "m"}, noPi, "pi is not on PATH"},
		{"ready", map[string]string{"PI_E2E": "1", "PI_E2E_MODEL": "m"}, havePi, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }
			got := realPiSkipReason(getenv, tc.look)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("skip reason = %q, want none", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("skip reason = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}
