import { describe, expect, it } from "vitest";
import { findFreePosition, positionsOverlap } from "./placement";

describe("findFreePosition", () => {
  const origin = { x: 0, y: 0 };

  it("keeps the preferred position when it is free", () => {
    expect(findFreePosition(origin, [{ x: 600, y: 400 }])).toEqual(origin);
  });

  it("moves away from an occupied preferred position", () => {
    const position = findFreePosition(origin, [origin]);
    expect(position).not.toEqual(origin);
    expect(positionsOverlap(position, origin)).toBe(false);
  });

  it("returns distinct, non-overlapping positions for successive additions", () => {
    const occupied = [origin];
    const placed = [];
    for (let index = 0; index < 6; index += 1) {
      const position = findFreePosition(origin, occupied);
      expect(occupied.some((taken) => positionsOverlap(position, taken))).toBe(
        false,
      );
      occupied.push(position);
      placed.push(`${position.x}:${position.y}`);
    }
    expect(new Set(placed).size).toBe(6);
  });

  it("uses only the available space, not the whole preferred axis", () => {
    const occupied = [
      { x: 0, y: 0 },
      { x: 0, y: 100 },
      { x: 244, y: 0 },
    ];
    const position = findFreePosition(origin, occupied);
    expect(occupied.some((taken) => positionsOverlap(position, taken))).toBe(
      false,
    );
  });
});
