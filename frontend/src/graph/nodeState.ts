import type { NodeRun, NodeStatus, Run } from "../types/run";
import { isTerminalRun } from "../types/run";

export type VisualState = NodeStatus | "waiting_answer" | "not_run";

// NodeStatus has no waiting_answer: the engine keeps the node running and
// records the question on the run. The visual state is therefore derived.
export function deriveNodeState(node: NodeRun | undefined, run: Run | null): VisualState {
  if (!node) return "not_run";

  if (run?.pending?.kind === "question" && run.pending.nodeId === node.nodeId) {
    return "waiting_answer";
  }
  if (run?.pending?.kind === "approval" && run.pending.nodeId === node.nodeId) {
    return "waiting_approval";
  }
  if (run && isTerminalRun(run.status) && node.status === "pending") {
    return "not_run";
  }
  return node.status;
}

export const NODE_STATE_LABEL: Record<VisualState, string> = {
  pending: "Pending",
  running: "Running",
  waiting_approval: "Waiting for approval",
  waiting_answer: "Waiting for answer",
  completed: "Completed",
  failed: "Failed",
  skipped: "Skipped",
  cancelled: "Cancelled",
  not_run: "Not run",
};

export function isWaitingState(state: VisualState): boolean {
  return state === "waiting_approval" || state === "waiting_answer";
}

export function isAttentionState(state: VisualState): boolean {
  return state === "failed" || isWaitingState(state);
}
