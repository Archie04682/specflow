import type { AskShape, RunEvent } from "../types/events";
import { truncate } from "../lib/format";

// The projection turns the wire stream into display items. It is deliberately
// pure: replay, reconnect, and tests all feed it the same events and get the
// same result. Token deltas never become a row each; they grow one block.

export const MAX_TEXT = 20_000;
export const MAX_SUMMARY = 16_000;
export const MAX_ARGS = 4_000;
// The DOM renders only the newest window of a long journal. The projection
// still keeps every item, and "Show earlier" widens the window on demand.
export const INITIAL_VISIBLE_ITEMS = 80;
export const EARLIER_PAGE_ITEMS = 120;

interface ItemBase {
  id: number;
  at: string;
  nodeId?: string;
}

export type StreamItem =
  | (ItemBase & {
      kind: "lifecycle";
      eventType: string;
      data: Record<string, unknown>;
    })
  | (ItemBase & {
      kind: "node";
      eventType: string;
      data: Record<string, unknown>;
    })
  | (ItemBase & { kind: "thinking"; text: string; complete: boolean })
  | (ItemBase & { kind: "message"; text: string; complete: boolean })
  | (ItemBase & {
      kind: "tool";
      callId: string;
      name: string;
      args?: Record<string, unknown>;
      summary?: string;
      state: "running" | "ok" | "error";
      durationMs?: number;
      files: string[];
    })
  | (ItemBase & {
      kind: "command";
      command: string;
      workdir?: string;
      state: "running" | "ok" | "error";
      exitCode?: number;
      output?: string;
      durationMs?: number;
    })
  | (ItemBase & { kind: "file"; path: string; tool?: string })
  | (ItemBase & { kind: "notice"; level: string; message: string })
  | (ItemBase & { kind: "status"; text: string })
  | (ItemBase & {
      kind: "interaction";
      eventType: string;
      data: Record<string, unknown>;
    })
  | (ItemBase & { kind: "unknown"; kindName: string });

export interface ProjectionState {
  items: StreamItem[];
  lastSeq: number;
  // The text block currently accepting deltas, keyed by node.
  open: Record<string, { kind: "thinking" | "text"; id: number }>;
  // Tool item id by callId, so updates and finishes find their card.
  tools: Record<string, number>;
  // Question shapes by question id. The run record carries the pending
  // interaction, but only this event carries the question kind.
  askById: Record<string, AskShape>;
}

export function createProjection(): ProjectionState {
  return { items: [], lastSeq: 0, open: {}, tools: {}, askById: {} };
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null
    ? (value as Record<string, unknown>)
    : undefined;
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function asNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function asBoolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function asStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const strings = value.filter((entry): entry is string => typeof entry === "string");
  return strings.length > 0 ? strings : undefined;
}

function argsPreview(args: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  if (!args) return undefined;
  let serialized: string;
  try {
    serialized = JSON.stringify(args);
  } catch {
    return undefined;
  }
  if (serialized.length <= MAX_ARGS) return args;
  return { truncated: truncate(serialized, MAX_ARGS) };
}

function replaceItem(
  state: ProjectionState,
  id: number,
  update: (item: StreamItem) => StreamItem,
): ProjectionState {
  // The item being updated is almost always the newest one (the open text
  // block or the tool card just started), so search backwards. This keeps a
  // long replay from turning every delta into a scan of the whole journal.
  for (let index = state.items.length - 1; index >= 0; index -= 1) {
    const current = state.items[index]!;
    if (current.id !== id) continue;
    const items = state.items.slice();
    items[index] = update(current);
    return { ...state, items };
  }
  return state;
}

function completeOpen(state: ProjectionState, nodeId: string | undefined): ProjectionState {
  if (!nodeId) return state;
  const entry = state.open[nodeId];
  if (!entry) return state;
  const next = replaceItem(state, entry.id, (item) =>
    item.kind === "thinking" || item.kind === "message" ? { ...item, complete: true } : item,
  );
  const open = { ...next.open };
  delete open[nodeId];
  return { ...next, open };
}

function appendDelta(
  state: ProjectionState,
  event: RunEvent,
  kind: "thinking" | "text",
  text: string,
): ProjectionState {
  if (!event.nodeId) return state;
  const nodeId = event.nodeId;
  const entry = state.open[nodeId];

  if (entry && entry.kind === kind) {
    const next = replaceItem(state, entry.id, (item) => {
      if (item.kind === "thinking" || item.kind === "message") {
        return { ...item, text: truncate(item.text + text, MAX_TEXT) };
      }
      return item;
    });
    return { ...next, lastSeq: event.seq };
  }

  const closed = completeOpen(state, nodeId);
  const item: StreamItem =
    kind === "thinking"
      ? {
          id: event.seq,
          at: event.at,
          nodeId,
          kind: "thinking",
          text: truncate(text, MAX_TEXT),
          complete: false,
        }
      : {
          id: event.seq,
          at: event.at,
          nodeId,
          kind: "message",
          text: truncate(text, MAX_TEXT),
          complete: false,
        };
  return {
    ...closed,
    items: [...closed.items, item],
    lastSeq: event.seq,
    open: { ...closed.open, [nodeId]: { kind, id: event.seq } },
  };
}

function finishDelta(
  state: ProjectionState,
  event: RunEvent,
  kind: "thinking" | "text",
  text: string | undefined,
): ProjectionState {
  if (!event.nodeId) return state;
  const nodeId = event.nodeId;
  const entry = state.open[nodeId];

  if (entry && entry.kind === kind) {
    const next = replaceItem(state, entry.id, (item) => {
      if (item.kind === "thinking" || item.kind === "message") {
        return { ...item, text: truncate(text ?? item.text, MAX_TEXT), complete: true };
      }
      return item;
    });
    const open = { ...next.open };
    delete open[nodeId];
    return { ...next, open, lastSeq: event.seq };
  }

  if (!text) return { ...state, lastSeq: event.seq };
  const closed = completeOpen(state, nodeId);
  const item: StreamItem =
    kind === "thinking"
      ? { id: event.seq, at: event.at, nodeId, kind: "thinking", text: truncate(text, MAX_TEXT), complete: true }
      : { id: event.seq, at: event.at, nodeId, kind: "message", text: truncate(text, MAX_TEXT), complete: true };
  return { ...closed, items: [...closed.items, item], lastSeq: event.seq };
}

function findTool(
  state: ProjectionState,
  callId: string,
): StreamItem | undefined {
  const id = state.tools[callId];
  if (id === undefined) return undefined;
  for (let index = state.items.length - 1; index >= 0; index -= 1) {
    const item = state.items[index]!;
    if (item.id === id) return item;
  }
  return undefined;
}

function upsertTool(
  state: ProjectionState,
  event: RunEvent,
  tool: Record<string, unknown>,
  phase: "start" | "update" | "finish",
): ProjectionState {
  const callId = asString(tool.callId) ?? `seq-${event.seq}`;
  const name = asString(tool.name) ?? "tool";
  const summary = asString(tool.summary);
  const isError = asBoolean(tool.isError) ?? false;
  const durationMs = asNumber(tool.durationMs);
  const existing = findTool(state, callId);

  if (existing && existing.kind === "tool") {
    const next = replaceItem(state, existing.id, (item) => {
      if (item.kind !== "tool") return item;
      return {
        ...item,
        name: item.name === "tool" ? name : item.name,
        summary: summary ? truncate(summary, MAX_SUMMARY) : item.summary,
        state: phase === "finish" ? (isError ? "error" : "ok") : item.state,
        durationMs: durationMs ?? item.durationMs,
      };
    });
    return { ...next, lastSeq: event.seq };
  }

  if (phase === "update" || phase === "finish") {
    // A missing start (truncated journal or an adapter quirk) still becomes a
    // card rather than a dangling update.
    const closed = completeOpen(state, event.nodeId);
    const item: StreamItem = {
      id: event.seq,
      at: event.at,
      nodeId: event.nodeId,
      kind: "tool",
      callId,
      name,
      summary: summary ? truncate(summary, MAX_SUMMARY) : undefined,
      state: phase === "finish" ? (isError ? "error" : "ok") : "running",
      durationMs,
      files: [],
    };
    return {
      ...closed,
      items: [...closed.items, item],
      lastSeq: event.seq,
      tools: { ...closed.tools, [callId]: event.seq },
    };
  }

  const closed = completeOpen(state, event.nodeId);
  const item: StreamItem = {
    id: event.seq,
    at: event.at,
    nodeId: event.nodeId,
    kind: "tool",
    callId,
    name,
    args: argsPreview(asRecord(tool.args)),
    state: "running",
    files: [],
  };
  return {
    ...closed,
    items: [...closed.items, item],
    lastSeq: event.seq,
    tools: { ...closed.tools, [callId]: event.seq },
  };
}

function attachFile(
  state: ProjectionState,
  event: RunEvent,
  path: string,
  toolName: string | undefined,
): ProjectionState {
  const nodeId = event.nodeId;
  if (nodeId && toolName) {
    for (let index = state.items.length - 1; index >= 0; index -= 1) {
      const item = state.items[index]!;
      if (item.kind !== "tool" || item.nodeId !== nodeId) continue;
      if (item.name !== toolName) continue;
      if (item.files.includes(path)) return { ...state, lastSeq: event.seq };
      const next = replaceItem(state, item.id, (candidate) =>
        candidate.kind === "tool"
          ? { ...candidate, files: [...candidate.files, path] }
          : candidate,
      );
      return { ...next, lastSeq: event.seq };
    }
  }
  const closed = completeOpen(state, nodeId);
  const item: StreamItem = {
    id: event.seq,
    at: event.at,
    nodeId,
    kind: "file",
    path,
    tool: toolName,
  };
  return { ...closed, items: [...closed.items, item], lastSeq: event.seq };
}

export function askShapeFromEvent(event: RunEvent): AskShape | null {
  if (event.type !== "question.asked" || !event.data) return null;
  const kind = asString(event.data.kind);
  if (!kind) return null;
  return {
    kind,
    title: asString(event.data.title),
    message: asString(event.data.message),
    options: asStringArray(event.data.options),
    default: asString(event.data.default),
    placeholder: asString(event.data.placeholder),
    prefill: asString(event.data.prefill),
    attempt: asNumber(event.data.attempt),
    deadline: asString(event.data.deadline),
  };
}

const NODE_EVENT_TYPES = new Set([
  "node.started",
  "node.completed",
  "node.failed",
  "node.postcondition_failed",
  "node.skipped",
]);

const INTERACTION_EVENT_TYPES = new Set([
  "question.asked",
  "question.answered",
  "question.timed_out",
  "question.cancelled",
  "approval.required",
  "approval.granted",
  "approval.rejected",
  "approval.timed_out",
  "injection.received",
  "injection.acknowledged",
  "injection.rejected",
]);

export function applyEvent(state: ProjectionState, event: RunEvent): ProjectionState {
  if (!Number.isFinite(event.seq) || event.seq <= state.lastSeq) {
    return state;
  }

  const data = event.data ?? {};

  if (event.type === "node.event") {
    const kind = asString(data.kind);
    if (!kind) {
      const closed = completeOpen(state, event.nodeId);
      return {
        ...closed,
        items: [
          ...closed.items,
          { id: event.seq, at: event.at, nodeId: event.nodeId, kind: "unknown", kindName: "node.event" },
        ],
        lastSeq: event.seq,
      };
    }

    switch (kind) {
      case "thinking_delta": {
        const text = asString(data.text) ?? "";
        return appendDelta(state, event, "thinking", text);
      }
      case "thinking_done":
        return finishDelta(state, event, "thinking", asString(data.text));
      case "text_delta": {
        const text = asString(data.text) ?? "";
        return appendDelta(state, event, "text", text);
      }
      case "text_done":
        return finishDelta(state, event, "text", asString(data.text));
      case "tool_started": {
        const tool = asRecord(data.tool);
        if (!tool) break;
        return upsertTool(state, event, tool, "start");
      }
      case "tool_updated": {
        const tool = asRecord(data.tool);
        if (!tool) break;
        return upsertTool(state, event, tool, "update");
      }
      case "tool_finished": {
        const tool = asRecord(data.tool);
        if (!tool) break;
        return upsertTool(state, event, tool, "finish");
      }
      case "file_changed": {
        const file = asRecord(data.file);
        if (!file) break;
        const path = asString(file.path);
        if (!path) break;
        return attachFile(state, event, path, asString(file.tool));
      }
      case "notice": {
        const notice = asRecord(data.notice);
        if (!notice) break;
        const message = asString(notice.message) ?? "notice";
        const closed = completeOpen(state, event.nodeId);
        return {
          ...closed,
          items: [
            ...closed.items,
            {
              id: event.seq,
              at: event.at,
              nodeId: event.nodeId,
              kind: "notice",
              level: asString(notice.level) ?? "info",
              message: truncate(message, MAX_SUMMARY),
            },
          ],
          lastSeq: event.seq,
        };
      }
      case "status": {
        const text = asString(data.text);
        if (!text) break;
        const closed = completeOpen(state, event.nodeId);
        return {
          ...closed,
          items: [
            ...closed.items,
            { id: event.seq, at: event.at, nodeId: event.nodeId, kind: "status", text: truncate(text, MAX_TEXT) },
          ],
          lastSeq: event.seq,
        };
      }
      case "usage":
      case "turn_started":
      case "turn_finished":
      case "injection_ack":
        return { ...state, lastSeq: event.seq };
      default:
        break;
    }

    const closed = completeOpen(state, event.nodeId);
    return {
      ...closed,
      items: [
        ...closed.items,
        { id: event.seq, at: event.at, nodeId: event.nodeId, kind: "unknown", kindName: kind },
      ],
      lastSeq: event.seq,
    };
  }

  if (event.type === "question.asked") {
    const closed = completeOpen(state, event.nodeId);
    const shape = askShapeFromEvent(event);
    const askById = shape && asString(data.id)
      ? { ...closed.askById, [asString(data.id)!]: shape }
      : closed.askById;
    return {
      ...closed,
      items: [
        ...closed.items,
        { id: event.seq, at: event.at, nodeId: event.nodeId, kind: "interaction", eventType: event.type, data },
      ],
      lastSeq: event.seq,
      askById,
    };
  }

  if (event.type === "command.started") {
    const closed = completeOpen(state, event.nodeId);
    const item: StreamItem = {
      id: event.seq,
      at: event.at,
      nodeId: event.nodeId,
      kind: "command",
      command: asString(data.command) ?? "(command not reported)",
      workdir: asString(data.workdir),
      state: "running",
    };
    return { ...closed, items: [...closed.items, item], lastSeq: event.seq };
  }

  if (event.type === "command.finished") {
    const exitCode = asNumber(data.exitCode);
    const output = asString(data.output);
    const durationMs = asNumber(data.durationMs);
    const settled: "ok" | "error" = exitCode === undefined || exitCode === 0 ? "ok" : "error";
    // Pair with the running card for this node; a replayed finish without its
    // start (truncated journal) still becomes a card instead of a lost result.
    for (let index = state.items.length - 1; index >= 0; index -= 1) {
      const item = state.items[index]!;
      if (item.kind !== "command" || item.nodeId !== event.nodeId) continue;
      if (item.state !== "running") continue;
      const next = replaceItem(state, item.id, (candidate) =>
        candidate.kind === "command"
          ? { ...candidate, state: settled, exitCode, output, durationMs }
          : candidate,
      );
      return { ...next, lastSeq: event.seq };
    }
    const closed = completeOpen(state, event.nodeId);
    const item: StreamItem = {
      id: event.seq,
      at: event.at,
      nodeId: event.nodeId,
      kind: "command",
      command: "(command not reported)",
      state: settled,
      exitCode,
      output,
      durationMs,
    };
    return { ...closed, items: [...closed.items, item], lastSeq: event.seq };
  }

  if (NODE_EVENT_TYPES.has(event.type)) {
    const closed = completeOpen(state, event.nodeId);
    return {
      ...closed,
      items: [
        ...closed.items,
        { id: event.seq, at: event.at, nodeId: event.nodeId, kind: "node", eventType: event.type, data },
      ],
      lastSeq: event.seq,
    };
  }

  if (INTERACTION_EVENT_TYPES.has(event.type)) {
    const closed = completeOpen(state, event.nodeId);
    return {
      ...closed,
      items: [
        ...closed.items,
        { id: event.seq, at: event.at, nodeId: event.nodeId, kind: "interaction", eventType: event.type, data },
      ],
      lastSeq: event.seq,
    };
  }

  const closed = completeOpen(state, event.nodeId);
  return {
    ...closed,
    items: [
      ...closed.items,
      { id: event.seq, at: event.at, nodeId: event.nodeId, kind: "lifecycle", eventType: event.type, data },
    ],
    lastSeq: event.seq,
  };
}

export function applyEvents(state: ProjectionState, events: RunEvent[]): ProjectionState {
  return events.reduce(applyEvent, state);
}

export function visibleItems(
  items: StreamItem[],
  visibleCount = INITIAL_VISIBLE_ITEMS,
): StreamItem[] {
  if (visibleCount >= items.length) return items;
  return items.slice(items.length - visibleCount);
}
