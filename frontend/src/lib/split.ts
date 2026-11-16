// The run view splits the graph and the activity column with one draggable
// separator. Widths are pixels: a percentage would let the graph's minimum
// drift with the viewport, and the operator is choosing a reading column.

export const DEFAULT_RUN_SIDE = 520;
export const MIN_RUN_SIDE = 360;
export const MAX_RUN_SIDE = 1600;
export const MIN_GRAPH = 320;
export const RUN_SPLIT_HANDLE = 8;
// `.run-body` is a three-column grid with a 12px gap on each side of the handle.
export const RUN_SPLIT_GAPS = 24;

/**
 * Clamps an activity-column width so the graph keeps its minimum and the
 * activity column keeps its own. A container too narrow for both minimums
 * keeps the activity minimum; the stacked layout takes over below the
 * desktop breakpoint before that can be seen.
 */
export function clampRunSide(width: number, containerWidth: number): number {
  const room =
    Number.isFinite(containerWidth) && containerWidth > 0
      ? containerWidth - MIN_GRAPH - RUN_SPLIT_HANDLE - RUN_SPLIT_GAPS
      : MAX_RUN_SIDE;
  const max = Math.max(MIN_RUN_SIDE, Math.min(MAX_RUN_SIDE, Math.floor(room)));
  const value = Number.isFinite(width) ? Math.round(width) : DEFAULT_RUN_SIDE;
  return Math.min(Math.max(value, MIN_RUN_SIDE), max);
}
