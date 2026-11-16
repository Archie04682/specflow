import { NODE_HEIGHT, NODE_WIDTH } from "../graph/layout";

export interface EditorPosition {
  x: number;
  y: number;
}

const MARGIN = 24;
const STEP_X = NODE_WIDTH + MARGIN;
const STEP_Y = NODE_HEIGHT + MARGIN;

const RING_OFFSETS: Array<[number, number]> = [
  [1, 0],
  [0, 1],
  [-1, 0],
  [0, -1],
  [1, 1],
  [-1, 1],
  [1, -1],
  [-1, -1],
  [2, 0],
  [0, 2],
  [-2, 0],
  [0, -2],
];

export function positionsOverlap(
  left: EditorPosition,
  right: EditorPosition,
): boolean {
  return (
    Math.abs(left.x - right.x) < NODE_WIDTH + MARGIN &&
    Math.abs(left.y - right.y) < NODE_HEIGHT + MARGIN
  );
}

/**
 * Finds a spot for a new node: the preferred point (the visible canvas center)
 * if free, otherwise the nearest slot on a ring around it. Positions supplied in
 * `occupied` are not mutated beyond the caller-owned list.
 */
export function findFreePosition(
  preferred: EditorPosition,
  occupied: EditorPosition[],
): EditorPosition {
  const isFree = (candidate: EditorPosition) =>
    !occupied.some((position) => positionsOverlap(candidate, position));

  if (isFree(preferred)) return preferred;

  for (const [dx, dy] of RING_OFFSETS) {
    const candidate = {
      x: preferred.x + dx * STEP_X,
      y: preferred.y + dy * STEP_Y,
    };
    if (isFree(candidate)) return candidate;
  }

  return { x: preferred.x + (occupied.length + 1) * STEP_X, y: preferred.y };
}
