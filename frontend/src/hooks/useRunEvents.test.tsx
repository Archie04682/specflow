import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import {
  FakeEventSource,
  fakeFactory,
  lastSource,
  settleStream,
} from "../test/fakes";
import { makeRun } from "../test/fixtures";
import { useRunStore } from "../stores/runStore";
import type { RunEvent } from "../types/events";
import { useRunEvents } from "./useRunEvents";

function event(
  seq: number,
  type: string,
  nodeId?: string,
  data?: Record<string, unknown>,
): RunEvent {
  return {
    seq,
    at: "2026-01-01T12:00:00Z",
    runId: "run-1",
    type,
    nodeId,
    data,
  };
}

function setup() {
  return renderHook(() =>
    useRunEvents({
      project: "/repo",
      runId: "run-1",
      factory: fakeFactory(),
    }),
  );
}

describe("useRunEvents", () => {
  beforeEach(() => {
    FakeEventSource.reset();
    useRunStore.getState().resetRun();
  });

  it("opens the run stream with the project and no initial after", () => {
    setup();
    expect(lastSource().url).toBe("/api/runs/run-1/events?project=%2Frepo");
  });

  it("reports connection states and dispatches named events", async () => {
    setup();
    const source = lastSource();

    act(() => source.open());
    expect(useRunStore.getState().streamStatus).toBe("live");

    act(() =>
      source.emit(
        "node.event",
        event(1, "node.event", "explore", { kind: "text_delta", text: "hello" }),
      ),
    );
    await settleStream();
    expect(useRunStore.getState().stream.items).toHaveLength(1);
  });

  it("folds a burst into one projection update", async () => {
    setup();
    const source = lastSource();
    act(() => source.open());

    let updates = 0;
    const unsubscribe = useRunStore.subscribe((state, previous) => {
      if (state.stream !== previous.stream) updates += 1;
    });

    act(() => {
      for (let seq = 1; seq <= 25; seq += 1) {
        source.emit(
          "node.event",
          event(seq, "node.event", "explore", {
            kind: "text_delta",
            text: "x",
          }),
        );
      }
    });

    expect(useRunStore.getState().stream.items).toHaveLength(0);
    await settleStream();
    unsubscribe();

    expect(updates).toBe(1);
    expect(useRunStore.getState().stream.items).toHaveLength(1);
  });

  it("reports a dropped transport as reconnecting", () => {
    setup();
    const source = lastSource();
    act(() => source.open());
    act(() => source.fail(0));
    expect(useRunStore.getState().streamStatus).toBe("reconnecting");
  });

  it("closes on the terminal event and does not reconnect", () => {
    setup();
    const source = lastSource();
    act(() => source.open());
    act(() =>
      source.emit("run.completed", event(2, "run.completed", undefined, { summary: "done" })),
    );

    expect(useRunStore.getState().streamStatus).toBe("ended");
    expect(source.closed).toBe(true);
    expect(FakeEventSource.instances).toHaveLength(1);
  });

  it("treats a closed transport on a terminal run as ended", () => {
    useRunStore.setState({ run: makeRun({ status: "completed" }) });
    setup();
    const source = lastSource();
    act(() => source.fail(2));
    expect(useRunStore.getState().streamStatus).toBe("ended");
  });

  it("treats a closed transport on a live run as a failure", () => {
    useRunStore.setState({ run: makeRun({ status: "running" }) });
    setup();
    const source = lastSource();
    act(() => source.fail(2));
    expect(useRunStore.getState().streamStatus).toBe("failed");
  });

  it("reconnects after the last applied sequence", async () => {
    const { result } = setup();
    const source = lastSource();
    act(() => source.open());
    act(() =>
      source.emit("node.started", event(5, "node.started", "explore", { title: "Explore" })),
    );
    await settleStream();

    act(() => result.current.reconnect());

    expect(FakeEventSource.instances).toHaveLength(2);
    expect(lastSource().url).toBe(
      "/api/runs/run-1/events?project=%2Frepo&after=5",
    );
  });

  it("ignores malformed event data", () => {
    setup();
    const source = lastSource();
    act(() => source.open());
    act(() => source.emitRaw("node.event", "not-json"));
    act(() => source.emitRaw("node.event", JSON.stringify({ type: "node.event" })));
    expect(useRunStore.getState().stream.items).toHaveLength(0);
  });

  it("does not duplicate items when the effect mounts twice", async () => {
    const first = renderHook(() =>
      useRunEvents({
        project: "/repo",
        runId: "run-1",
        factory: fakeFactory(),
      }),
    );

    const source = lastSource();
    const one = event(1, "node.event", "explore", {
      kind: "text_delta",
      text: "once",
    });
    act(() => source.emit("node.event", one));
    act(() => source.emit("node.event", one));
    await settleStream();
    expect(useRunStore.getState().stream.items).toHaveLength(1);

    // A second mount (what StrictMode's development double-invoke does) opens
    // after the applied sequence and replays nothing new.
    first.unmount();

    renderHook(() =>
      useRunEvents({
        project: "/repo",
        runId: "run-1",
        factory: fakeFactory(),
      }),
    );

    expect(lastSource().url).toBe(
      "/api/runs/run-1/events?project=%2Frepo&after=1",
    );
    act(() => lastSource().emit("node.event", one));
    await settleStream();
    expect(useRunStore.getState().stream.items).toHaveLength(1);
  });
});
