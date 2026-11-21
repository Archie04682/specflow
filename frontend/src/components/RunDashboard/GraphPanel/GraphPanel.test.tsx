import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { deriveGraphEdges } from "../../../graph/edgeState";
import { layoutGraph } from "../../../graph/layout";
import { graphNode, makeGraph, makeNode, makeRun } from "../../../test/fixtures";
import type { RunActions } from "../../../hooks/useRunActions";
import { GraphPanel } from "./GraphPanel";

const routedGraph = makeGraph({
  nodes: [
    graphNode("review", { title: "Review" }),
    graphNode("gate", { title: "Review gate", kind: "command" }),
    graphNode("fix", { title: "Fix" }),
    graphNode("verify", { title: "Verify" }),
  ],
  edges: [
    { from: "review", to: "gate" },
    { from: "gate", to: "fix", when: "issues" },
    { from: "gate", to: "verify", when: "clean" },
    { from: "fix", to: "verify" },
  ],
});

const actions = {
  retry: vi.fn(),
  cancel: vi.fn(),
  resume: vi.fn(),
  approve: vi.fn(),
  answer: vi.fn(),
  inject: vi.fn(),
} as unknown as RunActions;

// The gate took `clean`, so the fix branch never ran.
const cleanRun = makeRun({
  status: "completed",
  nodes: [
    makeNode({ nodeId: "review", status: "completed" }),
    makeNode({
      nodeId: "gate",
      kind: "command",
      status: "completed",
      outcome: "clean",
    }),
    makeNode({ nodeId: "fix", status: "skipped" }),
    makeNode({ nodeId: "verify", status: "completed" }),
  ],
});

const issuesRun = makeRun({
  status: "completed",
  nodes: [
    makeNode({ nodeId: "review", status: "completed" }),
    makeNode({
      nodeId: "gate",
      kind: "command",
      status: "completed",
      outcome: "issues",
    }),
    makeNode({ nodeId: "fix", status: "completed" }),
    makeNode({ nodeId: "verify", status: "completed" }),
  ],
});

function renderPanel(run = cleanRun) {
  return render(
    <div style={{ width: 900, height: 700 }}>
      <GraphPanel graph={routedGraph} graphError={null} run={run} actions={actions} />
    </div>,
  );
}

describe("GraphPanel routing", () => {
  it("mounts the canvas for the pinned graph", () => {
    renderPanel();

    expect(screen.getByText("4 nodes")).toBeInTheDocument();
    expect(screen.getByTestId("workflow-graph")).toBeInTheDocument();
  });

  it("reports a graph it cannot read instead of hiding the panel", () => {
    render(
      <GraphPanel
        graph={null}
        graphError="graph is unavailable"
        run={cleanRun}
        actions={actions}
      />,
    );

    expect(screen.getByText("Graph unavailable")).toBeInTheDocument();
    expect(screen.getByText("graph is unavailable")).toBeInTheDocument();
  });

  // React Flow draws an edge only after it has measured both endpoints, and
  // jsdom reports every element as 0x0, so the canvas renders no edges here.
  // The marking is therefore asserted on the projection the canvas draws from.
  it("marks the branch the gate did not take", () => {
    const edges = deriveGraphEdges(routedGraph, cleanRun);
    const byLabel = new Map(
      edges
        .filter((edge) => edge.label)
        .map((edge) => [edge.label as string, edge]),
    );

    expect(byLabel.get("issues")?.state).toBe("untaken");
    expect(byLabel.get("issues")?.source).toBe("gate");
    expect(byLabel.get("clean")?.state).toBe("taken");
  });

  it("marks the other branch when the gate routed to it", () => {
    const edges = deriveGraphEdges(routedGraph, issuesRun);
    const byLabel = new Map(
      edges
        .filter((edge) => edge.label)
        .map((edge) => [edge.label as string, edge]),
    );

    expect(byLabel.get("issues")?.state).toBe("taken");
    expect(byLabel.get("clean")?.state).toBe("untaken");
  });

  it("never moves a node when the branch changes", () => {
    const before = layoutGraph(routedGraph.nodes, routedGraph.edges);
    const after = layoutGraph(routedGraph.nodes, routedGraph.edges);

    expect(after).toEqual(before);

    const states = (run: typeof cleanRun) =>
      deriveGraphEdges(routedGraph, run).map((edge) => edge.state);
    expect(states(cleanRun)).not.toEqual(states(issuesRun));
  });
});
