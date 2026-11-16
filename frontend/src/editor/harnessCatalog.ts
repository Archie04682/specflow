import type { HarnessInfo, ModelOption } from "../types/harness";

// The editor keeps its pickers honest with three small derivations: resolve the
// harness in play, find the selected model, and compute the thinking levels that
// model actually supports.

export function resolveHarness(
  harnesses: HarnessInfo[] | null | undefined,
  id: string | undefined,
): HarnessInfo | null {
  if (!harnesses || harnesses.length === 0) return null;
  const wanted = id?.trim();
  if (wanted) {
    const match = harnesses.find((harness) => harness.id === wanted);
    if (match) return match;
  }
  return harnesses[0] ?? null;
}

export function findModel(
  harness: HarnessInfo | null,
  modelId: string | undefined,
): ModelOption | undefined {
  if (!harness || !modelId) return undefined;
  return harness.models?.find((model) => model.id === modelId);
}

export function thinkingLevelsFor(
  harness: HarnessInfo | null,
  modelId: string | undefined,
): { levels: string[]; reasoning: boolean } {
  const globalLevels = harness?.thinkingLevels?.length
    ? harness.thinkingLevels
    : ["off", "minimal", "low", "medium", "high", "xhigh", "max"];
  const model = findModel(harness, modelId);
  if (!model) {
    return { levels: globalLevels, reasoning: harness?.capabilities.thinking ?? true };
  }
  if (!model.reasoning) return { levels: ["off"], reasoning: false };
  return {
    levels: model.thinkingLevels?.length ? model.thinkingLevels : globalLevels,
    reasoning: true,
  };
}

// A model change must not leave a thinking level the new model does not
// understand; an unsupported value would only fail later, at run time.
export function thinkingForModelChange(
  harness: HarnessInfo | null,
  modelId: string | undefined,
  currentThinking: string | undefined,
): string | undefined {
  if (!currentThinking) return undefined;
  const { levels } = thinkingLevelsFor(harness, modelId);
  return levels.includes(currentThinking) ? currentThinking : undefined;
}
