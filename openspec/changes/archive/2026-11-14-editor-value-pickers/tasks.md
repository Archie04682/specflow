# Tasks

## 1. Harness catalog backend

- [x] 1.1 Add `harness.Catalog`, `ModelOption`, `ToolOption`, and the optional `CatalogProvider` interface; verify the existing `Adapter` interface and every adapter still compile unchanged.
- [x] 1.2 Implement the Pi catalog reader over `PI_CODING_AGENT_DIR` / `~/.pi/agent`: enabled-model filtering, provider-qualified ids, per-model thinking levels from `thinkingLevelMap`, global levels, default-model fallback, and built-in tool descriptions; verify missing or malformed files fall back instead of failing.
- [x] 1.3 Test the Pi catalog with a temporary agent directory: enabled filtering, reasoning/non-reasoning levels, tool list, no-config fallback, and default-model fallback.
- [x] 1.4 Extend the harness listing with optional `models`, `thinkingLevels`, `tools`, and non-fatal `catalogError`; verify a catalog failure still returns the harness entry and an adapter without the interface is unchanged.

## 2. Editor model and thinking

- [x] 2.1 Extend the frontend harness types with the catalog fields and add pure helpers for harness resolution, model lookup, thinking-level derivation, and model-change thinking cleanup; cover them with unit tests.
- [x] 2.2 Build the shared `HarnessModelFields` component: grouped model select, inherit and explicit-default options, preserved unknown values, catalog-driven thinking, disabled non-reasoning thinking, and the typed-identifier fallback when no catalog exists.
- [x] 2.3 Use the shared fields in the node inspector and the workflow defaults panel, wiring the active harness from the node or defaults; verify existing inspector/editor tests still pass with the fallback.
- [x] 2.4 Test selecting a model, narrowing thinking, dropping an unsupported level on model change, and disabling thinking for a non-reasoning model.

## 3. Tools multi-select

- [x] 3.1 Build the reusable `MultiSelectField`: trigger chips, grouped checkbox panel, custom-name entry, clear/Done actions, outside-click and Escape closing, and keyboard-visible focus.
- [x] 3.2 Use it for tools when the harness reports them and keep the comma-separated text fallback otherwise; test checking several tools, custom adding, clearing, and the fallback.

## 4. Prompt variables

- [x] 4.1 Add the closed prompt-variable constant mirroring `RunContext.Values`: `objective`, `change`, `run_id`, `workdir`, each with a description.
- [x] 4.2 Render the variables as chips under the prompt; insert `{{token}}` at the caret, keep focus, and explain that unknown placeholders fail compilation; test insertion and the complete set.

## 5. Verification

- [x] 5.1 Run the full frontend and Go suites, the frontend typecheck, and the single-binary build; verify they pass.
- [x] 5.2 Serve the built binary and confirm `/api/harnesses` on this machine reports the configured Pi model, thinking levels, and tools; open the editor, switch model/thinking, select tools and multiple skills from their dropdowns, insert a prompt variable, save and reload the draft; record screenshots under `evidence/`.
- [x] 5.3 Run `openspec validate editor-value-pickers --strict` and `openspec validate --changes --strict`; verify both pass.
- [x] 5.4 Update the README API/harness listing and editor sections plus AGENTS.md's adapter/catalog note; verify the documented fields and fallbacks match the running UI.
