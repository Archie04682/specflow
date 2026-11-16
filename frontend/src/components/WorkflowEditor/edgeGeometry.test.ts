import { Position } from "@xyflow/react";
import { describe, expect, it } from "vitest";
import { edgeAnchors } from "./edgeGeometry";

function rect(x: number, y: number) {
  return { x, y, width: 220, height: 76 };
}

describe("edgeAnchors", () => {
  it("routes horizontally when nodes are side by side", () => {
    expect(edgeAnchors(rect(0, 0), rect(400, 0))).toEqual({
      sourceX: 220,
      sourceY: 38,
      targetX: 400,
      targetY: 38,
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
    });
  });

  it("routes right to left", () => {
    expect(edgeAnchors(rect(400, 0), rect(0, 0))).toMatchObject({
      sourceX: 400,
      targetX: 220,
      sourcePosition: Position.Left,
      targetPosition: Position.Right,
    });
  });

  it("routes vertically when nodes are stacked", () => {
    expect(edgeAnchors(rect(0, 0), rect(0, 200))).toMatchObject({
      sourceY: 76,
      targetY: 200,
      sourcePosition: Position.Bottom,
      targetPosition: Position.Top,
    });
  });

  it("routes bottom to top", () => {
    expect(edgeAnchors(rect(0, 400), rect(0, 0))).toMatchObject({
      sourceY: 400,
      targetY: 76,
      sourcePosition: Position.Top,
      targetPosition: Position.Bottom,
    });
  });

  it("picks the dominant axis for diagonal placement", () => {
    expect(edgeAnchors(rect(0, 0), rect(400, 100)).sourcePosition).toBe(
      Position.Right,
    );
    expect(edgeAnchors(rect(0, 0), rect(100, 400)).sourcePosition).toBe(
      Position.Bottom,
    );
  });
});
