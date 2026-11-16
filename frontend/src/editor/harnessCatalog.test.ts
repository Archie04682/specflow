import { describe, expect, it } from "vitest";
import type { HarnessInfo } from "../types/harness";
import {
  findModel,
  resolveHarness,
  thinkingForModelChange,
  thinkingLevelsFor,
} from "./harnessCatalog";

function harness(overrides: Partial<HarnessInfo> = {}): HarnessInfo {
  return {
    id: "pi",
    name: "Pi",
    version: "1.0.0",
    protocol: 1,
    ready: true,
    capabilities: {
      streaming: true,
      thinking: true,
      toolEvents: true,
      fileEvents: true,
      questions: true,
      injection: true,
      abort: true,
      skills: true,
      modelSelect: true,
      toolPolicy: true,
      plugins: true,
      usage: true,
      sessionResume: false,
    },
    models: [
      {
        id: "deepseek/deepseek-flash",
        name: "DeepSeek Flash",
        provider: "deepseek",
        reasoning: true,
        thinkingLevels: ["off", "low", "high"],
      },
      {
        id: "plain/model",
        name: "Plain",
        provider: "plain",
        reasoning: false,
        thinkingLevels: ["off"],
      },
    ],
    thinkingLevels: ["off", "minimal", "low", "medium", "high"],
    tools: [{ name: "read" }],
    ...overrides,
  };
}

describe("harness catalog helpers", () => {
  it("resolves the requested harness or falls back to the first", () => {
    const pi = harness();
    expect(resolveHarness([pi], "pi")).toBe(pi);
    expect(resolveHarness([pi], "missing")).toBe(pi);
    expect(resolveHarness([], "pi")).toBeNull();
  });

  it("finds a model by id", () => {
    expect(findModel(harness(), "plain/model")?.name).toBe("Plain");
    expect(findModel(harness(), "missing")).toBeUndefined();
  });

  it("uses the model's levels when present and the harness levels otherwise", () => {
    expect(thinkingLevelsFor(harness(), "deepseek/deepseek-flash").levels).toEqual([
      "off",
      "low",
      "high",
    ]);
    expect(thinkingLevelsFor(harness(), "unknown").levels).toEqual([
      "off",
      "minimal",
      "low",
      "medium",
      "high",
    ]);
  });

  it("disables reasoning for a non-reasoning model", () => {
    expect(thinkingLevelsFor(harness(), "plain/model")).toEqual({
      levels: ["off"],
      reasoning: false,
    });
  });

  it("drops a thinking level the new model does not support", () => {
    expect(
      thinkingForModelChange(harness(), "deepseek/deepseek-flash", "high"),
    ).toBe("high");
    expect(
      thinkingForModelChange(harness(), "deepseek/deepseek-flash", "medium"),
    ).toBeUndefined();
    expect(thinkingForModelChange(harness(), "plain/model", "high")).toBeUndefined();
  });
});
