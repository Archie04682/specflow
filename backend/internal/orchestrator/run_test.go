package orchestrator

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRunStatusesRoundTripThroughJSON(t *testing.T) {
	for _, status := range AllRunStatuses {
		run := &Run{ID: "run-1", Status: status, StartedAt: time.Now()}

		data, err := json.Marshal(run)
		if err != nil {
			t.Fatalf("marshal %s: %v", status, err)
		}

		var decoded Run
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal %s: %v", status, err)
		}
		if decoded.Status != status {
			t.Fatalf("status %q came back as %q", status, decoded.Status)
		}
	}
}

func TestNodeStatusesRoundTripThroughJSON(t *testing.T) {
	for _, status := range AllNodeStatuses {
		node := NodeRun{NodeID: "n", Kind: NodeKindAgent, Status: status}

		data, err := json.Marshal(node)
		if err != nil {
			t.Fatalf("marshal %s: %v", status, err)
		}

		var decoded NodeRun
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal %s: %v", status, err)
		}
		if decoded.Status != status {
			t.Fatalf("status %q came back as %q", status, decoded.Status)
		}
	}
}

func TestRunContextValuesAreTheClosedSet(t *testing.T) {
	values := RunContext{
		Objective: "ship it",
		Change:    "add-export",
		RunID:     "run-9",
		Workdir:   "/tmp/wt",
	}.Values()

	want := map[string]string{
		"objective": "ship it",
		"change":    "add-export",
		"run_id":    "run-9",
		"workdir":   "/tmp/wt",
	}
	for key, expected := range want {
		if values[key] != expected {
			t.Fatalf("%s = %q, want %q", key, values[key], expected)
		}
	}
}

func TestTerminalRunStatuses(t *testing.T) {
	for _, status := range AllRunStatuses {
		want := status == RunCompleted || status == RunFailed || status == RunCancelled
		if status.Terminal() != want {
			t.Fatalf("%s.Terminal() = %v, want %v", status, status.Terminal(), want)
		}
	}
}

func TestPendingInteractionRoundTripsThroughJSON(t *testing.T) {
	deadline := time.Now().Add(5 * time.Minute).Truncate(time.Second)
	run := &Run{
		ID: "run-pending",
		Pending: &PendingInteraction{
			Kind:        "approval",
			ID:          "approval-1",
			NodeID:      "merge",
			Attempt:     2,
			Title:       "Merge",
			Action:      "git merge",
			RequestedAt: time.Now().Truncate(time.Second),
			Deadline:    &deadline,
		},
		Injections: []InjectionRecord{{
			ID: "i1", NodeID: "apply", Text: "use csv", Disposition: "delivered",
		}},
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Run
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Pending == nil || decoded.Pending.Attempt != 2 || decoded.Pending.Deadline == nil {
		t.Fatalf("pending = %+v", decoded.Pending)
	}
	if len(decoded.Injections) != 1 || decoded.Injections[0].Disposition != "delivered" {
		t.Fatalf("injections = %+v", decoded.Injections)
	}
}

func TestAppendInjectionCapsHistory(t *testing.T) {
	run := &Run{}
	for index := 0; index < maxInjectionRecords+5; index++ {
		run.appendInjection(InjectionRecord{ID: string(rune('a' + index%26))})
	}
	if len(run.Injections) != maxInjectionRecords {
		t.Fatalf("injections = %d, want %d", len(run.Injections), maxInjectionRecords)
	}
}
