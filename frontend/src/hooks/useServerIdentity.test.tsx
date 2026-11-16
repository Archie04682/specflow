import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { fakeResponse, stubFetch } from "../test/http";
import { useServerIdentity } from "./useServerIdentity";

describe("useServerIdentity", () => {
  it("reads the running server identity on mount", async () => {
    stubFetch(() =>
      fakeResponse(200, {
        service: "specflow",
        revision: "abc123",
        startedAt: "2026-11-15T19:12:00Z",
      }),
    );

    const { result } = renderHook(() => useServerIdentity());

    await waitFor(() => expect(result.current.identity?.revision).toBe("abc123"));
    expect(result.current.identity?.startedAt).toBe("2026-11-15T19:12:00Z");
    expect(result.current.loaded).toBe(true);
  });

  it("reports that it is not loaded before the first response", () => {
    stubFetch(() => new Promise<Response>(() => {}));

    const { result } = renderHook(() => useServerIdentity());

    expect(result.current.loaded).toBe(false);
    expect(result.current.identity).toBeNull();
  });

  it("refreshes the identity when the window regains focus", async () => {
    let calls = 0;
    stubFetch(() => {
      calls += 1;
      return fakeResponse(
        200,
        calls === 1
          ? { revision: "aaa", startedAt: "2026-11-15T10:00:00Z" }
          : { revision: "bbb", startedAt: "2026-11-15T11:00:00Z" },
      );
    });

    const { result } = renderHook(() => useServerIdentity());
    await waitFor(() => expect(result.current.identity?.revision).toBe("aaa"));

    act(() => {
      window.dispatchEvent(new Event("focus"));
    });

    await waitFor(() => expect(result.current.identity?.revision).toBe("bbb"));
  });

  it("leaves no error state when the identity read fails", async () => {
    stubFetch(() => Promise.reject(new Error("offline")));

    const { result } = renderHook(() => useServerIdentity());
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    await act(async () => {});

    expect(result.current.identity).toBeNull();
    expect(result.current.loaded).toBe(true);
  });
});
