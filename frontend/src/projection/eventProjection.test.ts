import { describe, expect, it } from "vitest";
import type { RunEvent } from "../types/events";
import {
  applyEvent,
  applyEvents,
  createProjection,
  visibleItems,
} from "./eventProjection";

function at(seq: number): string {
  return new Date(Date.UTC(2026, 0, 1, 12, 0, seq % 60)).toISOString();
}

function event(
  seq: number,
  type: string,
  data?: Record<string, unknown>,
  nodeId?: string,
): RunEvent {
  return { seq, at: at(seq), type, runId: "run-1", nodeId, data };
}

function nodeEvent(
  seq: number,
  kind: string,
  extra: Record<string, unknown> = {},
  nodeId = "explore",
): RunEvent {
  return event(seq, "node.event", { kind, ...extra }, nodeId);
}

describe("event projection", () => {
  it("coalesces thinking deltas and trusts the completion text", () => {
    const state = applyEvents(createProjection(), [
      nodeEvent(1, "thinking_delta", { text: "I " }),
      nodeEvent(2, "thinking_delta", { text: "think" }),
      nodeEvent(3, "thinking_done", { text: "I think so" }),
    ]);

    expect(state.items).toHaveLength(1);
    const item = state.items[0]!;
    expect(item.kind).toBe("thinking");
    if (item.kind === "thinking") {
      expect(item.text).toBe("I think so");
      expect(item.complete).toBe(true);
    }
  });

  it("closes an assistant block when a tool starts", () => {
    const state = applyEvents(createProjection(), [
      nodeEvent(1, "text_delta", { text: "Hello" }),
      nodeEvent(2, "text_delta", { text: " world" }),
      nodeEvent(3, "tool_started", { tool: { callId: "c1", name: "read" } }),
    ]);

    expect(state.items.map((item) => item.kind)).toEqual(["message", "tool"]);
    const message = state.items[0]!;
    expect(message.kind === "message" && message.complete).toBe(true);
    expect(message.kind === "message" && message.text).toBe("Hello world");
  });

  it("keeps one tool card through start, update, and finish", () => {
    const state = applyEvents(createProjection(), [
      nodeEvent(1, "tool_started", {
        tool: { callId: "c1", name: "read", args: { path: "main.go" } },
      }),
      nodeEvent(2, "tool_updated", {
        tool: { callId: "c1", name: "read", summary: "partial" },
      }),
      nodeEvent(3, "tool_finished", {
        tool: { callId: "c1", name: "read", summary: "done", durationMs: 12 },
      }),
    ]);

    expect(state.items).toHaveLength(1);
    const item = state.items[0]!;
    expect(item.kind).toBe("tool");
    if (item.kind === "tool") {
      expect(item.name).toBe("read");
      expect(item.summary).toBe("done");
      expect(item.state).toBe("ok");
      expect(item.durationMs).toBe(12);
      expect(item.args).toEqual({ path: "main.go" });
    }
  });

  it("replaces accumulated delta text with the completion text", () => {
    const state = applyEvents(createProjection(), [
      nodeEvent(1, "text_delta", { text: "abc" }),
      nodeEvent(2, "text_done", { text: "xyz" }),
    ]);

    const item = state.items[0]!;
    expect(item.kind === "message" && item.text).toBe("xyz");
  });

  it("attaches a file change to the matching tool card", () => {
    const state = applyEvents(createProjection(), [
      nodeEvent(1, "tool_started", { tool: { callId: "c1", name: "read" } }),
      nodeEvent(2, "tool_finished", { tool: { callId: "c1", name: "read" } }),
      nodeEvent(3, "file_changed", { file: { path: "main.go", tool: "read" } }),
    ]);

    expect(state.items).toHaveLength(1);
    const item = state.items[0]!;
    expect(item.kind === "tool" && item.files).toEqual(["main.go"]);
  });

  it("renders an unattached file change as its own row", () => {
    const state = applyEvents(createProjection(), [
      nodeEvent(1, "file_changed", { file: { path: "orphan.go", tool: "write" } }),
    ]);

    expect(state.items).toHaveLength(1);
    expect(state.items[0]!.kind).toBe("file");
  });

  it("never invents a known kind for an unknown node event", () => {
    const state = applyEvent(createProjection(), nodeEvent(1, "brand_new_kind", { text: "x" }));
    expect(state.items).toHaveLength(1);
    expect(state.items[0]!.kind).toBe("unknown");
  });

  it("survives an absent payload", () => {
    const state = applyEvent(createProjection(), event(1, "node.event", undefined, "explore"));
    expect(state.items).toHaveLength(1);
    expect(state.items[0]!.kind).toBe("unknown");
  });

  it("does not render usage or turn markers as rows", () => {
    const state = applyEvents(createProjection(), [
      nodeEvent(1, "turn_started"),
      nodeEvent(2, "usage", { usage: { totalTokens: 10 } }),
      nodeEvent(3, "turn_finished"),
    ]);

    expect(state.items).toHaveLength(0);
    expect(state.lastSeq).toBe(3);
  });

  it("records the question kind for the interaction dock", () => {
    const state = applyEvent(
      createProjection(),
      event(
        1,
        "question.asked",
        {
          id: "q1",
          kind: "confirm",
          title: "Continue?",
          message: "Use OAuth?",
          attempt: 2,
        },
        "propose",
      ),
    );

    expect(state.askById.q1).toMatchObject({
      kind: "confirm",
      title: "Continue?",
      message: "Use OAuth?",
      attempt: 2,
    });
    expect(state.items[0]!.kind).toBe("interaction");
  });

  it("is idempotent when a replay overlaps", () => {
    const events = [
      nodeEvent(1, "text_delta", { text: "a" }),
      nodeEvent(2, "text_delta", { text: "b" }),
      nodeEvent(3, "text_done", { text: "ab" }),
      nodeEvent(4, "tool_started", { tool: { callId: "c1", name: "read" } }),
    ];

    const first = applyEvents(createProjection(), events);
    const replayed = applyEvents(first, events);
    expect(replayed.items).toBe(first.items);

    const overlapping = applyEvent(replayed, nodeEvent(3, "text_done", { text: "ab" }));
    expect(overlapping.items).toBe(first.items);
  });

  it("accepts a reconnect from a later sequence without duplicating earlier items", () => {
    const prefix = [
      nodeEvent(1, "text_delta", { text: "a" }),
      nodeEvent(2, "text_done", { text: "a" }),
    ];
    const tail = [nodeEvent(3, "tool_started", { tool: { callId: "c1", name: "read" } })];

    const state = applyEvents(applyEvents(createProjection(), prefix), tail);
    expect(state.items).toHaveLength(2);
    expect(state.items[0]!.kind).toBe("message");
    expect(state.items[1]!.kind).toBe("tool");
  });

  it("bounds a token-rate burst to one item and a bounded window", () => {
    let state = createProjection();
    for (let seq = 1; seq <= 10_000; seq += 1) {
      state = applyEvent(state, nodeEvent(seq, "text_delta", { text: "x" }));
    }

    expect(state.items).toHaveLength(1);
    expect(visibleItems(state.items, 500)).toHaveLength(1);
  });

  it("windows a long item list to the most recent entries", () => {
    const items = Array.from({ length: 12 }, (_, index) => ({
      id: index + 1,
      at: at(index),
      kind: "status" as const,
      text: `s${index}`,
    }));

    const window = visibleItems(items, 5);
    expect(window).toHaveLength(5);
    expect(window[0]!.id).toBe(8);
    expect(window[4]!.id).toBe(12);
  });
});

describe("command events", () => {
  it("starts a running command card and settles it on finish", () => {
    const state = applyEvents(createProjection(), [
      event(1, "command.started", { command: "make test", workdir: "/repo" }, "verify"),
      event(
        2,
        "command.finished",
        { exitCode: 0, output: "ok", durationMs: 1200 },
        "verify",
      ),
    ]);

    expect(state.items).toHaveLength(1);
    const item = state.items[0]!;
    expect(item.kind).toBe("command");
    if (item.kind === "command") {
      expect(item.command).toBe("make test");
      expect(item.workdir).toBe("/repo");
      expect(item.state).toBe("ok");
      expect(item.exitCode).toBe(0);
      expect(item.output).toBe("ok");
      expect(item.durationMs).toBe(1200);
    }
  });

  it("marks a non-zero exit as failed", () => {
    const state = applyEvents(createProjection(), [
      event(1, "command.started", { command: "false" }, "verify"),
      event(2, "command.finished", { exitCode: 1, output: "boom" }, "verify"),
    ]);

    const item = state.items[0]!;
    expect(item.kind).toBe("command");
    if (item.kind === "command") expect(item.state).toBe("error");
  });

  it("renders a finish without its start as a settled card", () => {
    const state = applyEvent(
      createProjection(),
      event(1, "command.finished", { exitCode: 0, output: "late" }, "verify"),
    );

    const item = state.items[0]!;
    expect(item.kind).toBe("command");
    if (item.kind === "command") {
      expect(item.command).toBe("(command not reported)");
      expect(item.state).toBe("ok");
      expect(item.output).toBe("late");
    }
  });

  it("keeps command events out of other kinds", () => {
    const state = applyEvent(
      createProjection(),
      event(1, "command.started", { command: "ls" }, "verify"),
    );
    expect(state.items[0]!.kind).toBe("command");
    expect(state.lastSeq).toBe(1);
  });
});
