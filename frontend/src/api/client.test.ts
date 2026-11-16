import { describe, expect, it } from "vitest";
import {
  ApiError,
  request,
} from "./client";
import {
  answerQuestion,
  approveGate,
  cancelRun,
  createRun,
  injectClarification,
  resumeRun,
  retryRun,
} from "./runs";
import {
  callBody,
  callMethod,
  callUrl,
  fakeResponse,
  rawResponse,
  stubFetch,
} from "../test/http";

describe("api client", () => {
  it("parses a successful JSON response", async () => {
    stubFetch(() => fakeResponse(200, { status: "ok" }));
    await expect(request<{ status: string }>("/api/health")).resolves.toEqual({
      status: "ok",
    });
  });

  it("appends the project as a query parameter", async () => {
    const fetchMock = stubFetch(() => fakeResponse(200, {}));
    await request("/api/runs/run-1", { project: "/tmp/my repo" });

    expect(callUrl(fetchMock)).toBe(
      "/api/runs/run-1?project=%2Ftmp%2Fmy%20repo",
    );
  });

  it("keeps the server message on an error status", async () => {
    stubFetch(() => fakeResponse(400, { error: "project is not a git repository" }));

    await expect(request("/api/runs")).rejects.toMatchObject({
      name: "ApiError",
      status: 400,
      message: "project is not a git repository",
    });
  });

  it("keeps a run carried by a start failure", async () => {
    stubFetch(() =>
      fakeResponse(500, { error: "start failed", run: { id: "run-9" } }),
    );

    const error = await request("/api/runs").catch((cause: unknown) => cause);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).run?.id).toBe("run-9");
  });

  it("reports a network failure distinctly", async () => {
    stubFetch(() => {
      throw new TypeError("failed to fetch");
    });

    await expect(request("/api/runs")).rejects.toMatchObject({
      status: 0,
      message: "Cannot reach the SpecFlow server. Check that it is running.",
    });
  });

  it("rejects an unreadable success body", async () => {
    stubFetch(() => rawResponse(200, "not-json"));

    await expect(request("/api/runs")).rejects.toMatchObject({
      status: 200,
      message: "The server returned an unreadable response.",
    });
  });

  it("falls back to a generic message for an error without a body", async () => {
    stubFetch(() => rawResponse(404, ""));
    await expect(request("/api/runs/run-1")).rejects.toMatchObject({
      status: 404,
      message: "Request failed with status 404.",
    });
  });

  it("treats 204 as an empty result", async () => {
    stubFetch(() => rawResponse(204, ""));
    await expect(request("/api/anything")).resolves.toBeUndefined();
  });
});

describe("run api calls", () => {
  it("uses the plural answer and injection endpoints with exact payloads", async () => {
    const fetchMock = stubFetch(() => fakeResponse(200, { id: "i1", status: "queued" }));

    await answerQuestion("/repo", "run-1", {
      questionId: "q1",
      confirmed: true,
    });
    expect(callMethod(fetchMock, 0)).toBe("POST");
    expect(callUrl(fetchMock, 0)).toBe(
      "/api/runs/run-1/answers?project=%2Frepo",
    );
    expect(callBody(fetchMock, 0)).toEqual({ questionId: "q1", confirmed: true });

    await injectClarification("/repo", "run-1", "use encoding/csv");
    expect(callUrl(fetchMock, 1)).toBe(
      "/api/runs/run-1/injections?project=%2Frepo",
    );
    expect(callBody(fetchMock, 1)).toEqual({ text: "use encoding/csv" });
  });

  it("sends approval by node and attempt", async () => {
    const fetchMock = stubFetch(() => fakeResponse(200, { status: "ok" }));

    await approveGate("/repo", "run-1", "merge", 2, "reject");
    expect(callUrl(fetchMock, 0)).toBe(
      "/api/runs/run-1/approve?project=%2Frepo",
    );
    expect(callBody(fetchMock, 0)).toEqual({
      nodeId: "merge",
      decision: "reject",
      attempt: 2,
    });
  });

  it("sends retry mode, resume, and cancel", async () => {
    const fetchMock = stubFetch(() => fakeResponse(200, { status: "ok" }));

    await retryRun("/repo", "run-1", "apply", "clean");
    expect(callBody(fetchMock, 0)).toEqual({ nodeId: "apply", mode: "clean" });

    await resumeRun("/repo", "run-1");
    expect(callUrl(fetchMock, 1)).toBe("/api/runs/run-1/resume?project=%2Frepo");

    await cancelRun("/repo", "run-1");
    expect(callUrl(fetchMock, 2)).toBe("/api/runs/run-1/cancel?project=%2Frepo");
  });

  it("creates a run with project, workflow, and objective in the body", async () => {
    const fetchMock = stubFetch(() => fakeResponse(201, { id: "run-2" }));

    await createRun("/repo", "standard-feature", "Add an export endpoint");
    expect(callMethod(fetchMock, 0)).toBe("POST");
    expect(callUrl(fetchMock, 0)).toBe("/api/runs");
    expect(callBody(fetchMock, 0)).toEqual({
      project: "/repo",
      workflow: "standard-feature",
      objective: "Add an export endpoint",
    });
  });
});
