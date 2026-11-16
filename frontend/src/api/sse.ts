import type { RunEvent } from "../types/events";

// The event type list is the SSE `event:` name set from
// backend/internal/orchestrator/engine.go and the README event table.
// EventSource dispatches named events by type, so this list is the client's
// subscription contract: a type added on the backend must be added here.
export const EVENT_TYPES = [
  "run.started",
  "node.started",
  "node.event",
  "node.completed",
  "node.failed",
  "node.postcondition_failed",
  "node.skipped",
  "command.started",
  "command.finished",
  "run.completed",
  "run.failed",
  "run.cancelled",
  "question.asked",
  "question.answered",
  "question.timed_out",
  "question.cancelled",
  "injection.received",
  "injection.acknowledged",
  "injection.rejected",
  "approval.required",
  "approval.granted",
  "approval.rejected",
  "approval.timed_out",
  "change.resolved",
] as const;

export type KnownEventType = (typeof EVENT_TYPES)[number];

const TERMINAL_EVENTS = new Set<string>(["run.completed", "run.failed", "run.cancelled"]);

export function isTerminalEvent(type: string): boolean {
  return TERMINAL_EVENTS.has(type);
}

// node.event and command.* are high-frequency narrative telemetry; neither
// changes the run record between REST refetches and must not trigger one.
export function isLifecycleEvent(type: string): boolean {
  if (type === "node.event" || type.startsWith("command.")) return false;
  return (
    type.startsWith("run.") ||
    type.startsWith("node.") ||
    type.startsWith("question.") ||
    type.startsWith("approval.") ||
    type.startsWith("injection.") ||
    type === "change.resolved"
  );
}

export function runEventsUrl(project: string, runId: string, after?: number): string {
  const params = new URLSearchParams({ project });
  if (typeof after === "number" && after > 0) {
    params.set("after", String(after));
  }
  return `/api/runs/${encodeURIComponent(runId)}/events?${params.toString()}`;
}

export function parseRunEvent(message: MessageEvent): RunEvent | null {
  if (typeof message.data !== "string" || message.data.length === 0) return null;
  try {
    const parsed: unknown = JSON.parse(message.data);
    if (typeof parsed !== "object" || parsed === null) return null;
    const value = parsed as Record<string, unknown>;
    if (typeof value.seq !== "number" || typeof value.type !== "string") return null;
    if (typeof value.runId !== "string" || typeof value.at !== "string") return null;
    return parsed as RunEvent;
  } catch {
    return null;
  }
}
