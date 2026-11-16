import type { GraphNode } from "../types/graph";
import type {
  WorkflowDefaultsDraft,
  WorkflowDraft,
  WorkflowNodeDraft,
} from "../types/workflowDraft";

export const REFERENCE_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;

export function isValidReference(reference: string): boolean {
  return REFERENCE_PATTERN.test(reference);
}

/**
 * Derives the workflow file reference from its display name.
 * "Some new flow" -> "some-new-flow"; the result must satisfy REFERENCE_PATTERN.
 */
export function referenceFromName(name: string): string {
  return name
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 128)
    .replace(/-+$/g, "");
}

export function emptyDraft(reference = ""): WorkflowDraft {
  return {
    id: reference,
    name: reference || undefined,
    version: 1,
    defaults: { harness: "pi" },
    nodes: [],
    edges: [],
  };
}

export function draftSignature(draft: WorkflowDraft): string {
  return JSON.stringify(draft);
}

export function nextNodeId(draft: WorkflowDraft): string {
  let index = draft.nodes.length + 1;
  while (draft.nodes.some((node) => node.id === `node-${index}`)) {
    index += 1;
  }
  return `node-${index}`;
}

export interface AddNodeResult {
  draft: WorkflowDraft;
  nodeId: string;
}

export function addNode(draft: WorkflowDraft): AddNodeResult {
  const nodeId = nextNodeId(draft);
  const node: WorkflowNodeDraft = {
    id: nodeId,
    title: "New node",
    kind: "agent",
  };
  return {
    nodeId,
    draft: { ...draft, nodes: [...draft.nodes, node] },
  };
}

export function updateNode(
  draft: WorkflowDraft,
  nodeId: string,
  patch: Partial<WorkflowNodeDraft>,
): WorkflowDraft {
  return {
    ...draft,
    nodes: draft.nodes.map((node) =>
      node.id === nodeId ? { ...node, ...patch } : node,
    ),
  };
}

export function removeNode(draft: WorkflowDraft, nodeId: string): WorkflowDraft {
  return {
    ...draft,
    nodes: draft.nodes.filter((node) => node.id !== nodeId),
    edges: draft.edges.filter(
      (edge) => edge.from !== nodeId && edge.to !== nodeId,
    ),
  };
}

export function canAddEdge(
  draft: WorkflowDraft,
  from: string,
  to: string,
): boolean {
  if (!from || !to || from === to) return false;
  const known = new Set(draft.nodes.map((node) => node.id));
  if (!known.has(from) || !known.has(to)) return false;
  return !draft.edges.some((edge) => edge.from === from && edge.to === to);
}

export function addEdge(
  draft: WorkflowDraft,
  from: string,
  to: string,
): WorkflowDraft {
  if (!canAddEdge(draft, from, to)) return draft;
  return { ...draft, edges: [...draft.edges, { from, to }] };
}

export function edgeKey(edge: { from: string; to: string }): string {
  return `${edge.from}->${edge.to}`;
}

/** Rewires one edge. Returns null for a self-loop, unknown node, or duplicate. */
export function replaceEdge(
  draft: WorkflowDraft,
  current: { from: string; to: string },
  next: { from: string; to: string },
): WorkflowDraft | null {
  if (!next.from || !next.to || next.from === next.to) return null;
  const known = new Set(draft.nodes.map((node) => node.id));
  if (!known.has(next.from) || !known.has(next.to)) return null;
  const duplicate = draft.edges.some(
    (edge) =>
      !(edge.from === current.from && edge.to === current.to) &&
      edge.from === next.from &&
      edge.to === next.to,
  );
  if (duplicate) return null;
  return {
    ...draft,
    edges: draft.edges.map((edge) =>
      edge.from === current.from && edge.to === current.to
        ? { from: next.from, to: next.to }
        : edge,
    ),
  };
}

export function removeEdge(
  draft: WorkflowDraft,
  from: string,
  to: string,
): WorkflowDraft {
  return {
    ...draft,
    edges: draft.edges.filter((edge) => !(edge.from === from && edge.to === to)),
  };
}

export function updateWorkflowFields(
  draft: WorkflowDraft,
  patch: Partial<Pick<WorkflowDraft, "id" | "name" | "description" | "version">>,
): WorkflowDraft {
  return { ...draft, ...patch };
}

export function updateDefaults(
  draft: WorkflowDraft,
  patch: Partial<WorkflowDefaultsDraft>,
): WorkflowDraft {
  return { ...draft, defaults: { ...draft.defaults, ...patch } };
}

export function normalizeModel(model: {
  id?: string;
  thinking?: string;
}): { id?: string; thinking?: string } | undefined {
  const id = model.id?.trim() ?? "";
  const thinking = model.thinking?.trim() ?? "";
  if (!id && !thinking) return undefined;
  return {
    ...(id ? { id } : {}),
    ...(thinking ? { thinking } : {}),
  };
}

export function setWorkflowModel(
  draft: WorkflowDraft,
  model: { id?: string; thinking?: string },
): WorkflowDraft {
  const defaults = { ...draft.defaults };
  const normalized = normalizeModel(model);
  if (normalized) {
    defaults.model = normalized;
  } else {
    delete defaults.model;
  }
  return { ...draft, defaults };
}

export function setNodeModel(
  draft: WorkflowDraft,
  nodeId: string,
  model: { id?: string; thinking?: string },
): WorkflowDraft {
  const normalized = normalizeModel(model);
  return updateNode(draft, nodeId, {
    model: normalized,
  });
}

export function nodeTitle(draft: WorkflowDraft, nodeId: string): string {
  const title = draft.nodes
    .find((node) => node.id === nodeId)
    ?.title?.trim();
  return title && title.length > 0 ? title : "Untitled node";
}

export function editorGraphNodes(draft: WorkflowDraft): GraphNode[] {
  return draft.nodes.map((node) => ({
    id: node.id,
    title: node.title?.trim() || "Untitled node",
    kind: node.kind,
    interactive: node.interactive ?? draft.defaults.interactive ?? false,
    allowInject: node.allowInject ?? draft.defaults.allowInject ?? false,
    requireApproval: node.requireApproval ?? false,
  }));
}

export function triStateValue(value: boolean | undefined): string {
  if (value === undefined) return "";
  return value ? "true" : "false";
}

export function parseTriState(value: string): boolean | undefined {
  if (value === "") return undefined;
  return value === "true";
}

export function effectiveHarness(
  draft: WorkflowDraft,
  node: WorkflowNodeDraft,
): string {
  return node.harness ?? draft.defaults.harness ?? "";
}
