import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { fakeResponse, stubFetch } from "../test/http";
import { FakeEventSource, fakeFactory, lastSource } from "../test/fakes";
import { makeGraph, makeRun } from "../test/fixtures";
import { useRunStore } from "../stores/runStore";
import type { RunEvent } from "../types/events";
import { useRunData } from "./useRunData";

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

const run = makeRun({ id: "run-1" });
const graph = makeGraph({ runId: "run-1" });

function runCalls(mock: ReturnType<typeof stubFetch>): number {
  return mock.mock.calls.filter((call) =>
    String(call[0]).includes("/api/runs/run-1?"),
  ).length;
}

async function settle(milliseconds = 220): Promise<void> {
  await new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

describe("useRunData", () => {
  beforeEach(() => {
    FakeEventSource.reset();
    useRunStore.getState().resetRun();
  });

  it("loads the run and its graph once, then refetches only on lifecycle events", async () => {
    const fetchMock = stubFetch((url) => {
      if (url.includes("/graph")) return fakeResponse(200, graph);
      if (url.includes("/api/runs/run-1?")) return fakeResponse(200, run);
      return fakeResponse(404, { error: "not found" });
    });

    renderHook(() =>
      useRunData("/repo", "run-1", { eventSourceFactory: fakeFactory() }),
    );

    await waitFor(() => expect(useRunStore.getState().run?.id).toBe("run-1"));
    await waitFor(() => expect(FakeEventSource.instances).toHaveLength(1));
    const source = lastSource();
    act(() => source.open());

    expect(runCalls(fetchMock)).toBe(1);

    act(() => source.emit("node.started", event(1, "node.started", { title: "Explore" })));
    await settle();
    expect(runCalls(fetchMock)).toBe(2);

    for (let seq = 2; seq <= 8; seq += 1) {
      act(() =>
        source.emit(
          "node.event",
          event(seq, "node.event", { kind: "text_delta", text: "x" }),
        ),
      );
    }
    await settle();
    expect(runCalls(fetchMock)).toBe(2);
  });

  it("does not poll while nothing happens", async () => {
    const fetchMock = stubFetch((url) => {
      if (url.includes("/graph")) return fakeResponse(200, graph);
      return fakeResponse(200, run);
    });

    renderHook(() =>
      useRunData("/repo", "run-1", { eventSourceFactory: fakeFactory() }),
    );

    await waitFor(() => expect(useRunStore.getState().run?.id).toBe("run-1"));
    await settle(400);
    expect(runCalls(fetchMock)).toBe(1);

    act(() => lastSource().open());
    await settle(400);
    expect(runCalls(fetchMock)).toBe(1);
  });

  it("refetches immediately on the terminal event", async () => {
    const fetchMock = stubFetch((url) => {
      if (url.includes("/graph")) return fakeResponse(200, graph);
      return fakeResponse(200, run);
    });

    renderHook(() =>
      useRunData("/repo", "run-1", { eventSourceFactory: fakeFactory() }),
    );

    await waitFor(() => expect(useRunStore.getState().run?.id).toBe("run-1"));
    await waitFor(() => expect(FakeEventSource.instances).toHaveLength(1));
    act(() => lastSource().open());

    act(() =>
      lastSource().emit(
        "run.completed",
        event(9, "run.completed", { summary: "done" }),
      ),
    );

    await waitFor(() => expect(runCalls(fetchMock)).toBe(2));
  });

  it("keeps the run when only the graph fails", async () => {
    stubFetch((url) => {
      if (url.includes("/graph")) return fakeResponse(500, { error: "graph is gone" });
      return fakeResponse(200, run);
    });

    renderHook(() =>
      useRunData("/repo", "run-1", { eventSourceFactory: fakeFactory() }),
    );

    await waitFor(() => expect(useRunStore.getState().run?.id).toBe("run-1"));
    await waitFor(() =>
      expect(useRunStore.getState().graphError).toBe("graph is gone"),
    );
    expect(useRunStore.getState().graph).toBeNull();
  });
});
