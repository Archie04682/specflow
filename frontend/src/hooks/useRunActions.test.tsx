import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useRunStore } from "../stores/runStore";
import {
  callBody,
  callUrl,
  fakeResponse,
  stubFetch,
} from "../test/http";
import { useRunActions, useStartRun } from "./useRunActions";

function setup() {
  const refresh = vi.fn(async () => {});
  const reconnect = vi.fn();
  const hook = renderHook(() =>
    useRunActions({
      project: "/repo",
      runId: "run-1",
      refresh,
      reconnect,
    }),
  );
  return { ...hook, refresh, reconnect };
}

describe("useRunActions", () => {
  beforeEach(() => {
    useRunStore.getState().resetRun();
  });

  it("answers a question with the exact payload", async () => {
    const fetchMock = stubFetch(() => fakeResponse(200, { status: "ok" }));
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.answer({ questionId: "q1", confirmed: false });
    });

    expect(callUrl(fetchMock)).toBe(
      "/api/runs/run-1/answers?project=%2Frepo",
    );
    expect(callBody(fetchMock)).toEqual({ questionId: "q1", confirmed: false });
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("queues an injection and reports it as queued", async () => {
    const fetchMock = stubFetch(() =>
      fakeResponse(200, { id: "apply-1", status: "queued" }),
    );
    const { result } = setup();

    await act(async () => {
      await result.current.inject("use encoding/csv");
    });

    expect(callUrl(fetchMock)).toBe(
      "/api/runs/run-1/injections?project=%2Frepo",
    );
    expect(useRunStore.getState().notice).toContain("queued");
  });

  it("approves with node and attempt", async () => {
    const fetchMock = stubFetch(() => fakeResponse(200, { status: "ok" }));
    const { result } = setup();

    await act(async () => {
      await result.current.approve("merge", 3, "reject");
    });

    expect(callBody(fetchMock)).toEqual({
      nodeId: "merge",
      decision: "reject",
      attempt: 3,
    });
  });

  it("retries and resumes, reopening the stream after each", async () => {
    const fetchMock = stubFetch(() => fakeResponse(200, { status: "ok" }));
    const { result, reconnect } = setup();

    await act(async () => {
      await result.current.retry("apply", "clean");
    });
    expect(callBody(fetchMock, 0)).toEqual({ nodeId: "apply", mode: "clean" });

    await act(async () => {
      await result.current.resume();
    });
    expect(callUrl(fetchMock, 1)).toBe(
      "/api/runs/run-1/resume?project=%2Frepo",
    );
    expect(reconnect).toHaveBeenCalledTimes(2);
  });

  it("cancels the run", async () => {
    const fetchMock = stubFetch(() => fakeResponse(200, { status: "cancelled" }));
    const { result } = setup();

    await act(async () => {
      await result.current.cancel();
    });

    expect(callUrl(fetchMock)).toBe(
      "/api/runs/run-1/cancel?project=%2Frepo",
    );
    expect(useRunStore.getState().notice).toBe("Cancellation requested.");
  });

  it("closes a resolved interaction on 410 and refreshes", async () => {
    stubFetch(() => fakeResponse(410, { error: "interaction already resolved" }));
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.answer({ questionId: "q1", value: "yes" });
    });

    expect(useRunStore.getState().notice).toBe("This interaction already resolved.");
    expect(useRunStore.getState().error).toBeNull();
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("surfaces a 409 and refreshes the run", async () => {
    stubFetch(() => fakeResponse(409, { error: "run is not waiting" }));
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.inject("hello");
    });

    expect(useRunStore.getState().notice).toBe("run is not waiting");
    expect(useRunStore.getState().error).toBeNull();
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("disables a second submission while an action is in flight", async () => {
    let release: (() => void) | undefined;
    stubFetch(
      () =>
        new Promise((resolve) => {
          release = () => resolve(fakeResponse(200, { status: "ok" }));
        }),
    );
    const { result } = setup();

    let pending: Promise<boolean> | undefined;
    act(() => {
      pending = result.current.answer({ questionId: "q1", value: "yes" });
    });
    expect(useRunStore.getState().pendingAction).toBe("answer");

    await act(async () => {
      release?.();
      await pending;
    });
    expect(useRunStore.getState().pendingAction).toBeNull();
  });
});

describe("useStartRun", () => {
  beforeEach(() => {
    useRunStore.getState().resetRun();
  });

  it("returns a created run", async () => {
    stubFetch(() => fakeResponse(201, { id: "run-2" }));
    const { result } = renderHook(() => useStartRun());

    let outcome;
    await act(async () => {
      outcome = await result.current("/repo", "standard", "ship it");
    });

    expect(outcome).toMatchObject({ ok: true, run: { id: "run-2" } });
    expect(useRunStore.getState().pendingAction).toBeNull();
  });

  it("keeps the run from a start failure and records the error", async () => {
    stubFetch(() =>
      fakeResponse(500, { error: "harness missing", run: { id: "run-3" } }),
    );
    const { result } = renderHook(() => useStartRun());

    let outcome;
    await act(async () => {
      outcome = await result.current("/repo", "standard", "ship it");
    });

    expect(outcome).toMatchObject({ ok: false, run: { id: "run-3" } });
    expect(useRunStore.getState().entryError).toBe("harness missing");
    expect(useRunStore.getState().pendingAction).toBeNull();
  });
});
