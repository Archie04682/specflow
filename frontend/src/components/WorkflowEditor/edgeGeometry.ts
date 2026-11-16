import { Position } from "@xyflow/react";

export const FALLBACK_NODE_WIDTH = 220;
export const FALLBACK_NODE_HEIGHT = 76;

export interface EdgeRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface EdgeAnchors {
  sourceX: number;
  sourceY: number;
  targetX: number;
  targetY: number;
  sourcePosition: Position;
  targetPosition: Position;
}

/**
 * Connects two node rectangles from whichever sides face each other. Dominant
 * axis wins, so side-by-side nodes get horizontal edges and stacked nodes get
 * vertical ones. Pure so the routing is testable without a browser.
 */
export function edgeAnchors(source: EdgeRect, target: EdgeRect): EdgeAnchors {
  const sourceCX = source.x + source.width / 2;
  const sourceCY = source.y + source.height / 2;
  const targetCX = target.x + target.width / 2;
  const targetCY = target.y + target.height / 2;
  const dx = targetCX - sourceCX;
  const dy = targetCY - sourceCY;

  if (Math.abs(dx) >= Math.abs(dy)) {
    const right = dx >= 0;
    return {
      sourceX: right ? source.x + source.width : source.x,
      sourceY: sourceCY,
      targetX: right ? target.x : target.x + target.width,
      targetY: targetCY,
      sourcePosition: right ? Position.Right : Position.Left,
      targetPosition: right ? Position.Left : Position.Right,
    };
  }

  const down = dy >= 0;
  return {
    sourceX: sourceCX,
    sourceY: down ? source.y + source.height : source.y,
    targetX: targetCX,
    targetY: down ? target.y : target.y + target.height,
    sourcePosition: down ? Position.Bottom : Position.Top,
    targetPosition: down ? Position.Top : Position.Bottom,
  };
}
