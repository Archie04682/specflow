import type { NodeKind } from "./run";

// A node as the workflow author wrote it. These are read-only projections: the
// catalog and a run's graph both use them, and neither carries execution state.

export interface GraphNode {
  id: string;
  title?: string;
  kind: NodeKind;
  interactive: boolean;
  allowInject: boolean;
  requireApproval: boolean;
}

export interface GraphEdge {
  from: string;
  to: string;
}

export interface RunGraph {
  runId: string;
  workflowId?: string;
  name?: string;
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface WorkflowSummary {
  reference: string;
  path: string;
  name?: string;
  description?: string;
  version?: number;
  valid: boolean;
  problems?: string[];
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface WorkflowCatalogResponse {
  workflows: WorkflowSummary[];
}
