import { describe, expect, it } from "vitest";
import {
  DEFAULT_RUN_SIDE,
  MAX_RUN_SIDE,
  MIN_RUN_SIDE,
  MIN_GRAPH,
  RUN_SPLIT_GAPS,
  RUN_SPLIT_HANDLE,
  clampRunSide,
} from "./split";

describe("clampRunSide", () => {
  it("keeps a width that leaves room for both panes", () => {
    expect(clampRunSide(600, 1440)).toBe(600);
  });

  it("stops at the activity minimum", () => {
    expect(clampRunSide(120, 1440)).toBe(MIN_RUN_SIDE);
  });

  it("stops where the graph would drop below its minimum", () => {
    const containerWidth = MIN_GRAPH + RUN_SPLIT_HANDLE + RUN_SPLIT_GAPS + 572;
    expect(clampRunSide(1200, containerWidth)).toBe(572);
  });

  it("keeps the activity minimum when the container is too narrow for both", () => {
    expect(clampRunSide(600, 500)).toBe(MIN_RUN_SIDE);
  });

  it("never exceeds the stored maximum", () => {
    expect(clampRunSide(MAX_RUN_SIDE + 500, 4000)).toBe(MAX_RUN_SIDE);
  });

  it("falls back to the default for a non-finite width", () => {
    expect(clampRunSide(Number.NaN, 1440)).toBe(DEFAULT_RUN_SIDE);
  });

  it("rounds a fractional width", () => {
    expect(clampRunSide(612.6, 1440)).toBe(613);
  });
});
