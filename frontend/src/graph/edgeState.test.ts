import { describe, expect, it } from "vitest";
import { makeGraph, makeNode, makeRun, graphNode } from "../test/fixtures";
import { deriveEdgeState, deriveGraphEdges } from "./edgeState";
import { deriveNodeState } from "./nodeState";

const routedGraph = makeGraph({
  nodes: [
    graphNode("gate", { kind: "command" }),
    graphNode("fix"),
    graphNode("verify"),
  ],
  edges: [
    { from: "gate", to: "fix", when: "issues" },
    { from: "gate", to: "verify", when: "clean" },
    { from: "fix", to: "verify" },
  ],
});

function routedRun(outcome: string, fixStatus: "skipped" | "completed" = "skipped") {
  return makeRun({
    status: "completed",
    nodes: [
      makeNode({ nodeId: "gate", kind: "command", status: "completed", outcome }),
      makeNode({ nodeId: "fix", status: fixStatus }),
      makeNode({ nodeId: "verify", status: "completed" }),
    ],
  });
}

describe("deriveEdgeState", () => {
  it("takes only the edge matching the recorded route", () => {
    const run = routedRun("clean");
    const gate = run.nodes[0];
    expect(deriveEdgeState({ from: "gate", to: "fix", when: "issues" }, gate, run)).toBe(
      "untaken",
    );
    expect(
      deriveEdgeState({ from: "gate", to: "verify", when: "clean" }, gate, run),
    ).toBe("taken");
  });

  it("takes every edge of a completing node that declares no condition", () => {
    const run = routedRun("clean");
    const fix = run.nodes[1];
    expect(deriveEdgeState({ from: "fix", to: "verify" }, fix, run)).toBe("untaken");

    const completed = makeRun({
      status: "completed",
      nodes: [makeNode({ nodeId: "fix", status: "completed" })],
    });
    expect(
      deriveEdgeState({ from: "fix", to: "verify" }, completed.nodes[0], completed),
    ).toBe("taken");
  });

  it("reads a skipped source's edges as untaken", () => {
    const run = routedRun("clean");
    expect(deriveEdgeState({ from: "fix", to: "verify" }, run.nodes[1], run)).toBe(
      "untaken",
    );
  });

  it("keeps an edge pending while its source has not ended", () => {
    const run = makeRun({
      status: "running",
      nodes: [makeNode({ nodeId: "gate", kind: "command", status: "running" })],
    });
    expect(deriveEdgeState({ from: "gate", to: "fix", when: "issues" }, run.nodes[0], run)).toBe(
      "pending",
    );
  });
});

describe("deriveGraphEdges", () => {
  it("carries the condition and the taken state", () => {
    const edges = deriveGraphEdges(routedGraph, routedRun("clean"));
    expect(edges.map((edge) => [edge.source, edge.target, edge.label, edge.state])).toEqual([
      ["gate", "fix", "issues", "untaken"],
      ["gate", "verify", "clean", "taken"],
      ["fix", "verify", undefined, "untaken"],
    ]);
  });

  it("marks the issues branch when the gate routed to it", () => {
    const edges = deriveGraphEdges(routedGraph, routedRun("issues", "completed"));
    expect(edges[0]!.state).toBe("taken");
    expect(edges[1]!.state).toBe("untaken");
  });
});

describe("skipped branches read as not run", () => {
  it("reports a skipped branch node as skipped, not pending", () => {
    const run = routedRun("clean");
    const fix = run.nodes.find((node) => node.nodeId === "fix");

    // A terminal run does not turn its skipped branch back into "pending".
    expect(deriveNodeState(fix, run)).toBe("skipped");
    expect(deriveGraphEdges(routedGraph, run)[2]!.state).toBe("untaken");
  });
});
