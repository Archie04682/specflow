import { deriveNodeState, type VisualState } from "./nodeState";
import type { GraphEdge, RunGraph } from "../types/graph";
import type { NodeRun, Run } from "../types/run";

/** Whether an edge carried the run's path. */
export type EdgeState = "taken" | "untaken" | "pending";

/**
 * Derives one edge's state from the run record.
 *
 * A routing node takes only the edge whose condition matches the label it
 * completed under; every other edge out of it was not taken. A node that
 * declares no conditions takes all of its edges. An edge whose source has not
 * ended yet is pending, not untaken, so a live run never reads as a decision
 * it has not made.
 */
export function deriveEdgeState(
  edge: GraphEdge,
  source: NodeRun | undefined,
  run: Run | null,
): EdgeState {
  const state = deriveNodeState(source, run);
  if (state === "completed") {
    if (!edge.when) return "taken";
    return (source?.outcome ?? "") === edge.when ? "taken" : "untaken";
  }
  if (isEndedWithoutRunning(state)) return "untaken";
  return "pending";
}

/** A state in which the source will take no further edge. */
function isEndedWithoutRunning(state: VisualState): boolean {
  return state === "skipped" || state === "not_run" || state === "cancelled";
}

/** One run-graph edge as the canvas draws it. */
export interface RunGraphEdge {
  id: string;
  source: string;
  target: string;
  /** The condition the edge declares, drawn as its label. */
  label?: string;
  state: EdgeState;
  animated: boolean;
}

/**
 * Projects the run's graph onto the canvas: topology and conditions from the
 * pinned definition, taken/untaken from the run record. Positions are not part
 * of this projection, so a state change can never move a node.
 */
export function deriveGraphEdges(graph: RunGraph, run: Run): RunGraphEdge[] {
  const runNodes = new Map(run.nodes.map((node) => [node.nodeId, node]));
  return graph.edges.map((edge, index) => {
    const source = runNodes.get(edge.from);
    const target = runNodes.get(edge.to);
    return {
      id: `${edge.from}->${edge.to}-${index}`,
      source: edge.from,
      target: edge.to,
      ...(edge.when ? { label: edge.when } : {}),
      state: deriveEdgeState(edge, source, run),
      animated: run.pending?.nodeId === edge.to || target?.status === "running",
    };
  });
}
