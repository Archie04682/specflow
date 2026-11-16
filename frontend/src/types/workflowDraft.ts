import type { NodeKind } from "./run";

// The structured workflow draft transported by the editor API. It mirrors the
// YAML schema with camelCase JSON names; the server owns YAML generation, so
// this is the only shape the client parses or sends.

export interface ModelDraft {
  id?: string;
  thinking?: string;
}

export interface PostconditionDraft {
  type: string;
  path?: string;
  change?: string;
  description?: string;
}

export interface WorkflowDefaultsDraft {
  harness?: string;
  model?: ModelDraft;
  interactive?: boolean;
  allowInject?: boolean;
  askTimeout?: string;
  approvalTimeout?: string;
}

export interface WorkflowNodeDraft {
  id: string;
  title?: string;
  kind: NodeKind;
  harness?: string;
  skill?: string;
  skills?: string[];
  prompt?: string;
  command?: string;
  workdir?: string;
  model?: ModelDraft;
  interactive?: boolean;
  allowInject?: boolean;
  requireApproval?: boolean;
  timeout?: string;
  askTimeout?: string;
  approvalTimeout?: string;
  retry?: number;
  tools?: string[];
  postcondition?: PostconditionDraft;
}

export interface WorkflowEdgeDraft {
  from: string;
  to: string;
}

export interface WorkflowDraft {
  id: string;
  name?: string;
  description?: string;
  version?: number;
  defaults: WorkflowDefaultsDraft;
  nodes: WorkflowNodeDraft[];
  edges: WorkflowEdgeDraft[];
}

export interface WorkflowSource {
  reference: string;
  path: string;
  source: string;
  hash: string;
  isTemplate: boolean;
  draft: WorkflowDraft;
}

export interface SaveWorkflowResult {
  path: string;
  hash: string;
}
