import { beforeEach, describe, expect, it } from "vitest";
import { useRunStore } from "./runStore";
import { makeRun } from "../test/fixtures";
import type { RunEvent } from "../types/events";

function event(seq: number, type: string, data?: Record<string, unknown>): RunEvent {
  return {
    seq,
    at: "2026-01-01T12:00:00Z",
    runId: "run-1",
    type,
    nodeId: "explore",
    data,
  };
}

describe("run store", () => {
  beforeEach(() => {
    useRunStore.getState().resetRun();
  });

  it("holds the authoritative run and graph separately from the stream", () => {
    const run = makeRun();
    const store = useRunStore.getState();

    store.setRun(run);
    store.setGraph({
      runId: run.id,
      nodes: [],
      edges: [],
    });

    expect(useRunStore.getState().run).toBe(run);
    expect(useRunStore.getState().graph?.runId).toBe("run-1");
    expect(useRunStore.getState().stream.items).toHaveLength(0);
  });

  it("ingests an event into the stream projection and advances the sequence", () => {
    useRunStore
      .getState()
      .ingestEvent(event(1, "node.event", { kind: "text_delta", text: "hello" }));

    const state = useRunStore.getState();
    expect(state.stream.lastSeq).toBe(1);
    expect(state.stream.items).toHaveLength(1);
    expect(state.stream.items[0]!.kind).toBe("message");
  });

  it("ignores a replayed event that is not newer", () => {
    const ingest = useRunStore.getState().ingestEvent;
    const first = event(2, "node.event", { kind: "text_delta", text: "a" });
    ingest(first);
    const before = useRunStore.getState().stream;

    ingest(first);
    ingest(event(1, "node.event", { kind: "text_delta", text: "older" }));

    expect(useRunStore.getState().stream).toBe(before);
  });

  it("ingests a replay batch in one projection update", () => {
    const store = useRunStore.getState();
    const before = store.stream;

    store.ingestEvents([
      event(1, "node.event", { kind: "text_delta", text: "hello" }),
      event(2, "node.event", { kind: "text_delta", text: " world" }),
    ]);

    const state = useRunStore.getState();
    expect(state.stream).not.toBe(before);
    expect(state.stream.lastSeq).toBe(2);
    expect(state.stream.items).toHaveLength(1);
    expect(state.stream.items[0]).toMatchObject({
      kind: "message",
      text: "hello world",
    });
  });

  it("ignores an empty replay batch", () => {
    const store = useRunStore.getState();
    const before = store.stream;
    store.ingestEvents([]);
    expect(useRunStore.getState().stream).toBe(before);
  });

  it("resets run state and the stream together", () => {
    const store = useRunStore.getState();
    store.setRun(makeRun());
    store.ingestEvent(event(1, "run.started", {}));
    store.selectNode("explore");

    useRunStore.getState().resetRun();

    const state = useRunStore.getState();
    expect(state.run).toBeNull();
    expect(state.graph).toBeNull();
    expect(state.selectedNodeId).toBeNull();
    expect(state.stream.items).toHaveLength(0);
    expect(state.stream.lastSeq).toBe(0);
  });

  it("keeps the entry error across a reset so a start failure survives navigation", () => {
    useRunStore.getState().setEntryError("start failed");
    useRunStore.getState().resetRun();
    expect(useRunStore.getState().entryError).toBe("start failed");
  });
});
