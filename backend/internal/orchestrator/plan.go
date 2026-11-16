package orchestrator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.com/specflow/backend/internal/harness"
)

// Duration is a time.Duration that marshals as a duration string. Plans are
// JSON-facing so they can be inspected without a CLI; nanoseconds are not a
// useful representation there.
type Duration time.Duration

// Duration returns the value as a time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// String implements fmt.Stringer using the compact form a workflow author
// writes: "20m", "1h30m", not time.Duration's "20m0s" or "1h30m0s".
func (d Duration) String() string {
	duration := time.Duration(d)
	if duration == 0 {
		return "0s"
	}

	sign := ""
	if duration < 0 {
		sign = "-"
		duration = -duration
	}

	hours := duration / time.Hour
	duration %= time.Hour
	minutes := duration / time.Minute
	duration %= time.Minute
	seconds := duration / time.Second
	remainder := duration % time.Second

	var text strings.Builder
	text.WriteString(sign)
	if hours > 0 {
		fmt.Fprintf(&text, "%dh", hours)
	}
	if minutes > 0 {
		fmt.Fprintf(&text, "%dm", minutes)
	}
	switch {
	case remainder > 0 && (seconds > 0 || hours > 0 || minutes > 0):
		value := float64(seconds) + float64(remainder)/float64(time.Second)
		text.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
		text.WriteString("s")
	case seconds > 0:
		fmt.Fprintf(&text, "%ds", seconds)
	case remainder > 0:
		text.WriteString(remainder.String())
	}
	return text.String()
}

// MarshalJSON renders the duration as "20m".
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON parses a duration string.
func (d *Duration) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("duration %q: %w", text, err)
	}
	*d = Duration(parsed)
	return nil
}

// WorkflowPlan is a validated workflow with every effective value resolved.
// The engine executes a plan; it never consults workflow defaults or resolves
// a skill name again.
type WorkflowPlan struct {
	ID          string     `json:"id"`
	Name        string     `json:"name,omitempty"`
	Description string     `json:"description,omitempty"`
	Version     int        `json:"version,omitempty"`
	Source      string     `json:"source,omitempty"`
	Nodes       []PlanNode `json:"nodes"`
	Order       []string   `json:"order"`
}

// PlanNode is one node with its effective values.
type PlanNode struct {
	ID              string          `json:"id"`
	Title           string          `json:"title,omitempty"`
	Kind            NodeKind        `json:"kind"`
	Harness         string          `json:"harness,omitempty"`
	Skill           *harness.Skill  `json:"skill,omitempty"`
	Skills          []harness.Skill `json:"skills,omitempty"`
	Model           *harness.Model  `json:"model,omitempty"`
	Interactive     bool            `json:"interactive"`
	AllowInject     bool            `json:"allowInject"`
	Workdir         string          `json:"workdir"`
	Timeout         Duration        `json:"timeout"`
	AskTimeout      Duration        `json:"askTimeout,omitempty"`
	ApprovalTimeout Duration        `json:"approvalTimeout,omitempty"`
	Retry           int             `json:"retry,omitempty"`
	Tools           []string        `json:"tools,omitempty"`
	Prompt          string          `json:"prompt,omitempty"`
	Command         string          `json:"command,omitempty"`
	RequireApproval bool            `json:"requireApproval,omitempty"`
	Postcondition   *Postcondition  `json:"postcondition,omitempty"`
}

// Node returns the plan node with an identifier.
func (p *WorkflowPlan) Node(id string) *PlanNode {
	return findNode(p.Nodes, id, func(node PlanNode) string { return node.ID })
}
