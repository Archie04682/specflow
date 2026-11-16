import { describe, expect, it } from "vitest";
import { graphNode } from "../test/fixtures";
import { GAP_Y, NODE_HEIGHT, layoutGraph } from "./layout";

describe("graph layout", () => {
  it("places a chain in one column", () => {
    const nodes = [graphNode("a"), graphNode("b"), graphNode("c")];
    const edges = [
      { from: "a", to: "b" },
      { from: "b", to: "c" },
    ];

    const positions = layoutGraph(nodes, edges);
    expect(positions.a).toMatchObject({ x: 0, y: 0 });
    expect(positions.b).toMatchObject({ x: 0, y: NODE_HEIGHT + GAP_Y });
    expect(positions.c).toMatchObject({ x: 0, y: (NODE_HEIGHT + GAP_Y) * 2 });
  });

  it("places a diamond's siblings on the same level, side by side", () => {
    const nodes = [graphNode("a"), graphNode("b"), graphNode("c"), graphNode("d")];
    const edges = [
      { from: "a", to: "b" },
      { from: "a", to: "c" },
      { from: "b", to: "d" },
      { from: "c", to: "d" },
    ];

    const positions = layoutGraph(nodes, edges);
    expect(positions.b!.y).toBe(positions.c!.y);
    expect(positions.b!.x).not.toBe(positions.c!.x);
    expect(positions.d!.y).toBeGreaterThan(positions.b!.y);
    expect(positions.d!.x).toBeGreaterThan(positions.b!.x);
    expect(positions.d!.x).toBeLessThan(positions.c!.x);
  });

  it("puts two roots on one level", () => {
    const positions = layoutGraph([graphNode("a"), graphNode("b")], []);
    expect(positions.a!.y).toBe(positions.b!.y);
    expect(positions.a!.x).not.toBe(positions.b!.x);
  });

  it("falls back to declaration order on a cycle", () => {
    const positions = layoutGraph(
      [graphNode("a"), graphNode("b")],
      [
        { from: "a", to: "b" },
        { from: "b", to: "a" },
      ],
    );
    expect(positions.a).toMatchObject({ x: 0, y: 0 });
    expect(positions.b).toMatchObject({ x: 0, y: NODE_HEIGHT + GAP_Y });
  });
});
