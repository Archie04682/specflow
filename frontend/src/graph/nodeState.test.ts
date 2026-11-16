import { describe, expect, it } from "vitest";
import { makeNode, makeRun } from "../test/fixtures";
import { deriveNodeState } from "./nodeState";

describe("node state derivation", () => {
  it("derives waiting for answer from a pending question", () => {
    const run = makeRun({
      status: "waiting_answer",
      pending: {
        kind: "question",
        id: "q1",
        nodeId: "explore",
        requestedAt: "2026-01-01T12:00:00Z",
      },
    });
    const node = makeNode({ status: "running" });

    expect(deriveNodeState(node, run)).toBe("waiting_answer");
  });

  it("uses the recorded waiting_for_approval node status", () => {
    const run = makeRun({
      status: "waiting_approval",
      pending: {
        kind: "approval",
        id: "a1",
        nodeId: "merge",
        requestedAt: "2026-01-01T12:00:00Z",
      },
    });
    const node = makeNode({ nodeId: "merge", status: "waiting_approval" });

    expect(deriveNodeState(node, run)).toBe("waiting_approval");
  });

  it("reads pending nodes in a terminal run as not run", () => {
    const run = makeRun({ status: "failed" });
    const node = makeNode({ status: "pending" });
    expect(deriveNodeState(node, run)).toBe("not_run");
  });

  it("keeps failed and completed as recorded", () => {
    const run = makeRun({ status: "failed" });
    expect(deriveNodeState(makeNode({ status: "failed" }), run)).toBe("failed");
    expect(deriveNodeState(makeNode({ status: "completed" }), run)).toBe("completed");
  });
});
