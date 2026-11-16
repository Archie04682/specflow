import type { GraphEdge, GraphNode } from "../types/graph";

// Layered, deterministic placement: roots at level 0, every node one level
// below its deepest predecessor, declaration order breaking ties. Positions
// depend only on topology, so status updates never move the graph.

export const NODE_WIDTH = 220;
export const NODE_HEIGHT = 76;
export const GAP_X = 48;
export const GAP_Y = 64;

export interface PositionedNode {
  id: string;
  x: number;
  y: number;
}

export type GraphPositions = Record<string, PositionedNode>;

export function layoutGraph(nodes: GraphNode[], edges: GraphEdge[]): GraphPositions {
  const order = nodes.map((node) => node.id);
  const indegree = new Map<string, number>(order.map((id) => [id, 0]));
  const outgoing = new Map<string, string[]>();

  for (const edge of edges) {
    if (!indegree.has(edge.from) || !indegree.has(edge.to)) continue;
    outgoing.set(edge.from, [...(outgoing.get(edge.from) ?? []), edge.to]);
    indegree.set(edge.to, (indegree.get(edge.to) ?? 0) + 1);
  }

  const level = new Map<string, number>();
  const queue = order.filter((id) => (indegree.get(id) ?? 0) === 0);
  for (const id of queue) level.set(id, 0);

  let processed = 0;
  while (queue.length > 0) {
    const id = queue.shift()!;
    processed += 1;
    for (const next of outgoing.get(id) ?? []) {
      level.set(next, Math.max(level.get(next) ?? 0, (level.get(id) ?? 0) + 1));
      const remaining = (indegree.get(next) ?? 1) - 1;
      indegree.set(next, remaining);
      if (remaining === 0) queue.push(next);
    }
  }

  if (processed !== nodes.length) {
    // A cycle is rejected before a run exists; if an edited file introduces
    // one, fall back to a readable declaration-order column instead of looping.
    return Object.fromEntries(
      order.map((id, index) => [
        id,
        { id, x: 0, y: index * (NODE_HEIGHT + GAP_Y) },
      ]),
    );
  }

  const byLevel = new Map<number, string[]>();
  for (const id of order) {
    const bucket = byLevel.get(level.get(id) ?? 0) ?? [];
    bucket.push(id);
    byLevel.set(level.get(id) ?? 0, bucket);
  }

  const positions: GraphPositions = {};
  const levels = [...byLevel.entries()].sort(([left], [right]) => left - right);
  const widths = levels.map(
    ([, ids]) => ids.length * NODE_WIDTH + (ids.length - 1) * GAP_X,
  );
  const widest = Math.max(...widths);

  levels.forEach(([depth, ids], levelIndex) => {
    const width = widths[levelIndex]!;
    const startX = (widest - width) / 2;
    ids.forEach((id, index) => {
      positions[id] = {
        id,
        x: startX + index * (NODE_WIDTH + GAP_X),
        y: depth * (NODE_HEIGHT + GAP_Y),
      };
    });
  });
  return positions;
}
