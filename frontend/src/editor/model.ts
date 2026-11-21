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
    nodes: draft.nodes.map((node) => {
      if (node.id !== nodeId) return node;
      const next = { ...node, ...patch };
      // Routes select on an exit code, so a kind without one cannot keep them.
      if (next.kind !== "command") delete next.routes;
      return next;
    }),
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
  current: { from: string; to: string; when?: string },
  next: { from: string; to: string; when?: string },
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
  // A condition survives only while its new source declares it, so rewiring to
  // a node without that route can never leave an invalid edge behind.
  const when =
    next.when && declaresRouteLabel(draft, next.from, next.when)
      ? next.when
      : undefined;
  return {
    ...draft,
    edges: draft.edges.map((edge) => {
      if (!(edge.from === current.from && edge.to === current.to)) return edge;
      return when
        ? { from: next.from, to: next.to, when }
        : { from: next.from, to: next.to };
    }),
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

/** The routes of one node, ordered by exit code for stable rendering. */
export function routeEntries(
  node: WorkflowNodeDraft,
): { code: number; label: string }[] {
  const routes = node.routes ?? {};
  return Object.keys(routes)
    .map((key) => ({ code: Number(key), label: routes[key] ?? "" }))
    .filter((entry) => Number.isFinite(entry.code))
    .sort((left, right) => left.code - right.code);
}

/** Sets one exit code's label, returning the node's whole route map. */
export function withRoute(
  node: WorkflowNodeDraft,
  code: number,
  label: string,
): Record<string, string> {
  return { ...(node.routes ?? {}), [String(code)]: label };
}

/** Removes one exit code, dropping the map entirely when it empties. */
export function withoutRoute(
  node: WorkflowNodeDraft,
  code: number,
): Record<string, string> | undefined {
  const routes = { ...(node.routes ?? {}) };
  delete routes[String(code)];
  return Object.keys(routes).length > 0 ? routes : undefined;
}

/** The distinct labels a node declares, sorted, for an edge's picker. */
export function routeLabels(draft: WorkflowDraft, nodeId: string): string[] {
  const node = draft.nodes.find((candidate) => candidate.id === nodeId);
  if (!node?.routes) return [];
  const labels = Object.values(node.routes)
    .map((label) => label.trim())
    .filter((label) => label.length > 0);
  return Array.from(new Set(labels)).sort();
}

/** Reports whether a node declares a route label. */
export function declaresRouteLabel(
  draft: WorkflowDraft,
  nodeId: string,
  label: string,
): boolean {
  return routeLabels(draft, nodeId).includes(label);
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

export interface CanvasEdge {
  id: string;
  from: string;
  to: string;
  /** The condition drawn on the edge, absent when the edge is always taken. */
  label?: string;
}

/**
 * Projects the draft's edges onto the canvas. The condition is carried as the
 * edge's label, so a routed workflow reads without opening its edge forms.
 */
export function canvasEdges(draft: WorkflowDraft): CanvasEdge[] {
  return draft.edges.map((edge) => ({
    id: edgeKey(edge),
    from: edge.from,
    to: edge.to,
    ...(edge.when ? { label: edge.when } : {}),
  }));
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
