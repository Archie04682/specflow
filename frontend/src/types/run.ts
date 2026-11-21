// Wire types for the run record. These mirror the Go JSON in
// backend/internal/orchestrator/run.go; two fields are always present even
// when empty because Go marshals structs (time.Time, Usage) without honoring
// omitempty: `startedAt` (possibly the zero time) and `usage` (possibly {}).

export type RunStatus =
  | "pending"
  | "running"
  | "waiting_approval"
  | "waiting_answer"
  | "completed"
  | "failed"
  | "cancelled";

export type NodeStatus =
  | "pending"
  | "running"
  | "waiting_approval"
  | "completed"
  | "failed"
  | "skipped"
  | "cancelled";

export type NodeKind = "agent" | "command";

export interface Usage {
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  totalTokens?: number;
  costUsd?: number;
}

export interface NodeRun {
  nodeId: string;
  title?: string;
  kind: NodeKind;
  status: NodeStatus;
  attempts?: number;
  text?: string;
  error?: string;
  failureClass?: string;
  exitCode?: number;
  /** The rendered command a `command` node ran; empty for agent nodes. */
  command?: string;
  output?: string;
  checkpoint?: string;
  /** The route label a routing node completed under, absent otherwise. */
  outcome?: string;
  /** Always present; an empty object when the harness reported nothing. */
  usage: Usage;
  /** Always present; "0001-01-01T00:00:00Z" when the node never started. */
  startedAt: string;
  endedAt?: string;
}

export type PendingKind = "question" | "approval";

export interface PendingInteraction {
  kind: PendingKind;
  id: string;
  nodeId: string;
  attempt?: number;
  title?: string;
  message?: string;
  options?: string[];
  action?: string;
  requestedAt: string;
  deadline?: string;
}

export interface InjectionRecord {
  id: string;
  nodeId: string;
  text: string;
  receivedAt: string;
  disposition?: string;
}

export interface Run {
  id: string;
  project: string;
  workflowId?: string;
  workflowPath?: string;
  workflowSnapshot?: string;
  status: RunStatus;
  objective: string;
  change?: string;
  changeBaseline?: string[];
  nodes: NodeRun[];
  checkpoints?: Record<string, string>;
  pending?: PendingInteraction;
  injections?: InjectionRecord[];
  vcs?: "git" | "folder";
  apply?: "pending" | "applied" | "discarded";
  sourceBranch?: string;
  baseCommit?: string;
  branch?: string;
  worktree?: string;
  currentNode?: string;
  error?: string;
  startedAt: string;
  finishedAt?: string;
}

export function isTerminalRun(status: RunStatus): boolean {
  return status === "completed" || status === "failed" || status === "cancelled";
}

export function totalUsage(nodes: NodeRun[]): { tokens: number; cost: number } {
  let tokens = 0;
  let cost = 0;
  for (const node of nodes) {
    tokens += node.usage.totalTokens ?? 0;
    cost += node.usage.costUsd ?? 0;
  }
  return { tokens, cost };
}
