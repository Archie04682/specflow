import type { Usage } from "./run";

// RunEvent is the SSE envelope from backend/internal/orchestrator/events.go.
// `type` is the dotted lifecycle name or "node.event"; harness telemetry is
// nested in `data` and discriminated by `data.kind`.
export interface RunEvent {
  seq: number;
  at: string;
  type: string;
  runId: string;
  nodeId?: string;
  data?: Record<string, unknown>;
}

export interface ToolEvent {
  callId?: string;
  name?: string;
  args?: Record<string, unknown>;
  summary?: string;
  isError?: boolean;
  durationMs?: number;
}

export interface FileChange {
  path: string;
  tool?: string;
}

export interface Notice {
  level: "info" | "warning" | "error";
  code?: string;
  message: string;
}

export interface InjectionAck {
  id: string;
  disposition?: string;
  error?: string;
}

/** The shape carried by a question.asked event and used by the answer dock. */
export interface AskShape {
  kind: string;
  title?: string;
  message?: string;
  options?: string[];
  default?: string;
  placeholder?: string;
  prefill?: string;
  attempt?: number;
  deadline?: string;
}

export type { Usage };
