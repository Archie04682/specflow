package pi

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"example.com/specflow/backend/internal/harness"
)

// TestPiSmoke runs one real execution against an installed Pi.
//
// It is opt-in because it needs the binary, credentials, and network. It is the
// only test that proves the adapter matches the installed Pi's protocol; the
// rest run against the scripted stand-in.
func TestPiSmoke(t *testing.T) {
	if os.Getenv("PI_SMOKE") != "1" {
		t.Skip("set PI_SMOKE=1 to run against a real pi executable")
	}

	binary, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pi is not installed")
	}

	adapter := New(Options{
		Binary:         binary,
		CacheDir:       t.TempDir(),
		StartupTimeout: 30 * time.Second,
		CommandTimeout: 60 * time.Second,
		ShutdownGrace:  5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	probe, err := adapter.Probe(ctx)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	host := &recordingHost{}
	result, err := adapter.Run(ctx, harness.Request{
		NodeID:  "smoke",
		Prompt:  "Reply with exactly the word ok. Do not use any tools.",
		Workdir: t.TempDir(),
	}, host)
	if err != nil {
		t.Fatalf("run against pi %s: %v", probe.RuntimeVersion, err)
	}
	if result.Status != harness.StatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if strings.TrimSpace(result.Text) == "" {
		t.Fatal("pi returned no text")
	}

	t.Logf("pi %s replied %q after %d turns (%d tokens, %d events)",
		probe.RuntimeVersion, strings.TrimSpace(result.Text), result.Turns,
		result.Usage.TotalTokens, len(host.recorded()))
}
