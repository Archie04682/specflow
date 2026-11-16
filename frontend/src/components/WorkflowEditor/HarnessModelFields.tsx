import type { ModelDraft } from "../../types/workflowDraft";
import type { HarnessInfo } from "../../types/harness";
import {
  findModel,
  resolveHarness,
  thinkingForModelChange,
  thinkingLevelsFor,
} from "../../editor/harnessCatalog";

interface HarnessModelFieldsProps {
  idPrefix: string;
  model: ModelDraft;
  harnesses: HarnessInfo[] | null;
  harnessId?: string;
  harnessesError?: string | null;
  scope: "node" | "defaults";
  onChange: (model: { id?: string; thinking?: string }) => void;
}

// Model and thinking share one rule set: the model list comes from the harness
// catalog, and the thinking list is narrowed by the selected model. When the
// adapter reports no catalog, both fields fall back to typed identifiers so an
// unknown adapter is not blocked by the editor.
export function HarnessModelFields({
  idPrefix,
  model,
  harnesses,
  harnessId,
  harnessesError,
  scope,
  onChange,
}: HarnessModelFieldsProps) {
  const harness = resolveHarness(harnesses, harnessId);
  const catalogModels = harness?.models ?? [];
  const modelId = model.id ?? "";
  const hasCatalog = catalogModels.length > 0;
  const levels = thinkingLevelsFor(harness, modelId);
  const harnessLabel = harness?.name ?? harnessId ?? "the harness";
  const modelFieldHint = hasCatalog
    ? findModel(harness, modelId)
      ? `Reported by ${harnessLabel}.`
      : "Choose the model this node should run, or let it inherit."
    : harnessesError
      ? "Could not read the harness catalog; enter a model identifier."
      : harness
        ? `${harnessLabel} reports no model catalog; enter a model identifier.`
        : "Enter a model identifier; it is forwarded to the harness.";

  const inheritedLabel =
    scope === "node" ? "Inherit workflow default" : "Harness default";

  const changeModel = (nextId: string) => {
    const nextThinking = thinkingForModelChange(
      harness,
      nextId || undefined,
      model.thinking,
    );
    onChange({
      id: nextId || undefined,
      thinking: nextThinking,
    });
  };

  const changeThinking = (nextThinking: string) => {
    onChange({
      id: model.id,
      thinking: nextThinking || undefined,
    });
  };

  const hasThinkingCatalog =
    harness !== null && (harness.thinkingLevels?.length ?? 0) > 0;
  const unknownModel = modelId !== "" && modelId !== "default" && !hasCatalog;
  const currentNotInCatalog =
    hasCatalog &&
    modelId !== "" &&
    modelId !== "default" &&
    !catalogModels.some((entry) => entry.id === modelId);

  return (
    <>
      <div className="field-grid">
        <div className="field">
          <label className="field__label" htmlFor={`${idPrefix}-model-id`}>
            Model
          </label>
          {hasCatalog ? (
            <>
              <select
                id={`${idPrefix}-model-id`}
                className="input mono"
                value={modelId}
                aria-describedby={`${idPrefix}-model-hint`}
                onChange={(event) => changeModel(event.target.value)}
              >
                <option value="">{inheritedLabel}</option>
                <option value="default">Explicit default</option>
                {currentNotInCatalog ? (
                  <option value={modelId}>{modelId} (not in catalog)</option>
                ) : null}
                {groupModelsByProvider(catalogModels).map((group) => (
                  <optgroup key={group.provider} label={group.provider}>
                    {group.models.map((entry) => (
                      <option key={entry.id} value={entry.id}>
                        {entry.name ?? entry.id}
                        {entry.reasoning ? " · reasoning" : ""}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
              <p
                className="field__hint"
                id={`${idPrefix}-model-hint`}
                title={modelFieldHint}
              >
                {modelFieldHint}
              </p>
            </>
          ) : (
            <>
              <input
                id={`${idPrefix}-model-id`}
                className="input mono"
                placeholder="default"
                value={modelId}
                aria-describedby={`${idPrefix}-model-hint`}
                onChange={(event) =>
                  onChange({ id: event.target.value, thinking: model.thinking })
                }
              />
              <p className="field__hint" id={`${idPrefix}-model-hint`}>
                {modelFieldHint}
              </p>
            </>
          )}
        </div>

        <div className="field">
          <label className="field__label" htmlFor={`${idPrefix}-model-thinking`}>
            Thinking
          </label>
          {hasThinkingCatalog ? (
            <>
              <select
                id={`${idPrefix}-model-thinking`}
                className="input mono"
                value={model.thinking ?? ""}
                disabled={!levels.reasoning || harness?.capabilities.thinking === false}
                aria-describedby={`${idPrefix}-thinking-hint`}
                onChange={(event) => changeThinking(event.target.value)}
              >
                <option value="">{inheritedLabel}</option>
                {levels.levels.map((level) => (
                  <option key={level} value={level}>
                    {level}
                  </option>
                ))}
                {model.thinking &&
                !levels.levels.includes(model.thinking) ? (
                  <option value={model.thinking}>
                    {model.thinking} (unsupported)
                  </option>
                ) : null}
              </select>
              <p className="field__hint" id={`${idPrefix}-thinking-hint`}>
                {!levels.reasoning
                  ? "This model does not report reasoning support."
                  : harness?.capabilities.thinking === false
                    ? "This harness does not support thinking levels."
                    : `Supported: ${levels.levels.join(", ")}.`}
              </p>
            </>
          ) : (
            <>
              <input
                id={`${idPrefix}-model-thinking`}
                className="input mono"
                placeholder="high"
                value={model.thinking ?? ""}
                aria-describedby={`${idPrefix}-thinking-hint`}
                onChange={(event) =>
                  onChange({ id: model.id, thinking: event.target.value })
                }
              />
              <p className="field__hint" id={`${idPrefix}-thinking-hint`}>
                Enter a reasoning level; it is forwarded to the harness.
              </p>
            </>
          )}
        </div>
      </div>

      {unknownModel && harnessesError ? (
        <p className="field__error" role="status">
          Model catalog unavailable: {harnessesError}
        </p>
      ) : null}
    </>
  );
}

function groupModelsByProvider(
  models: NonNullable<HarnessInfo["models"]>,
): { provider: string; models: NonNullable<HarnessInfo["models"]> }[] {
  const groups = new Map<string, NonNullable<HarnessInfo["models"]>>();
  for (const model of models) {
    const provider = model.provider?.trim() || "models";
    const bucket = groups.get(provider);
    if (bucket) bucket.push(model);
    else groups.set(provider, [model]);
  }
  return [...groups.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([provider, entries]) => ({ provider, models: entries }));
}
