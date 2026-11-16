import { describe, expect, it } from "vitest";
import {
  ApiError,
} from "./client";
import {
  getWorkflowSource,
  saveWorkflow,
} from "./workflowEditor";
import type { WorkflowDraft } from "../types/workflowDraft";
import { callBody, callUrl, fakeResponse, stubFetch } from "../test/http";

const draft: WorkflowDraft = {
  id: "demo",
  name: "Demo",
  defaults: {},
  nodes: [{ id: "node-1", kind: "agent", title: "Explore" }],
  edges: [],
};

describe("workflow editor api", () => {
  it("reads a workflow with the project parameter", async () => {
    const fetchMock = stubFetch(() =>
      fakeResponse(200, {
        reference: "demo",
        path: "/repo/.specflow/workflows/demo.yaml",
        source: "id: demo\n",
        hash: "hash-1",
        isTemplate: true,
        draft,
      }),
    );

    const source = await getWorkflowSource("/repo", "a b");
    expect(callUrl(fetchMock)).toBe(
      "/api/workflows/a%20b?project=%2Frepo",
    );
    expect(source.hash).toBe("hash-1");
    expect(source.isTemplate).toBe(true);
  });

  it("saves the draft with the base hash", async () => {
    const fetchMock = stubFetch(() =>
      fakeResponse(200, {
        path: "/repo/.specflow/workflows/demo.yaml",
        hash: "hash-2",
      }),
    );

    await saveWorkflow("/repo", "demo", draft, "hash-1");
    expect(callUrl(fetchMock)).toBe(
      "/api/workflows/demo?project=%2Frepo",
    );
    expect(callBody(fetchMock)).toEqual({ draft, baseHash: "hash-1" });
  });

  it("surfaces a conflict as an ApiError", async () => {
    stubFetch(() =>
      fakeResponse(409, { error: "workflow changed on disk", hash: "hash-2" }),
    );

    const error = await saveWorkflow("/repo", "demo", draft, "hash-1").catch(
      (cause: unknown) => cause,
    );
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(409);
  });
});
