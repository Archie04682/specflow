# Proposal

## Why

The node inspector asks the operator to remember exact model identifiers, reasoning-level spellings, and tool names as free text. A typo is only discovered when the run compiles, and there is no surface that says which values the installed harness actually supports. The prompt field has the same problem in reverse: the server renders a closed set of run values, but the editor never shows them, so `{{objective}}` and friends have to be recalled from documentation.

The values already exist in the harness. Pi carries a model catalog with reasoning metadata and a set of tools; the orchestrator already owns the prompt variable contract and rejects unknown placeholders. The editor should expose those facts instead of making the operator type them.

## What Changes

- Extend the optional harness catalog surfaced through `GET /api/harnesses`:
  - models with `id`, display `name`, `provider`, `reasoning`, and the thinking levels that model supports;
  - the harness-wide thinking levels as a fallback;
  - tool names with short descriptions;
  - a non-fatal `catalogError` when a catalog cannot be read.
- Implement the first catalog provider in the Pi adapter by reading the local Pi agent configuration (`settings.json`, `models-store.json`, optional `models.json`). It does not start a runtime or execute work.
- Turn the node and workflow-default **Model** and **Thinking** fields into selects driven by that catalog:
  - a model change drops a thinking level the new model does not support;
  - thinking is disabled when the selected model reports no reasoning support;
  - adapters without a catalog keep the current typed-identifier fallback.
- Replace the comma-separated **Tools** field with a multi-select dropdown when the catalog reports tools: the trigger shows selected tools as chips, the panel offers checkboxes, a custom-name input covers unlisted tools, and an explicit clear action preserves the semantics that an empty allowlist means "harness default".
- Add **Prompt variables** under the prompt field: `{{objective}}`, `{{change}}`, `{{run_id}}`, and `{{workdir}}` as clickable chips that insert at the caret. The list mirrors the server's closed `RunContext` set; no template language is added.
- Make **Skills** a multi-select dropdown grouped by source, with selected skills shown as chips. A node stores a `skills:` list; the older singular `skill:` field stays readable, and a node that sets both is rejected.
- Resolve every selected skill during compilation, carry all of them to the adapter, and have Pi load each path. Pi can only expand one `/skill:` command, so when more than one skill is selected the adapter inlines every skill's instructions before the node prompt.
- Fallbacks stay honest: if the catalog is missing or the adapter reports no models, the editor says so and keeps manual entry.

**Not included:** a new run engine or graph model, a different prompt renderer, adapter lifecycle or transport changes, model discovery through provider logins, or a free-form template engine.

## Capabilities

### New Capabilities

- `harness-catalog`: the optional model/thinking/tool catalog an adapter reports through the harness listing, and how the API keeps a catalog failure non-fatal.
- `editor-value-pickers`: the node and defaults model/thinking selects, the multi-select tool and skill controls, and the prompt-variable chips, including their fallbacks.
- `multi-skill-nodes`: the plural `skills` workflow field, compile/request propagation, and Pi's multi-skill prompt composition.

## Impact

- `backend/internal/harness`: optional `CatalogProvider` types and a plural `Request.Skills` binding; the `Adapter` interface is unchanged.
- `backend/internal/harness/pi`: the catalog reader, multi-path `--skill` loading, and multi-skill prompt composition.
- `backend/internal/orchestrator`: plural `skills` workflow field and draft round-trip, full resolution into the plan, request propagation, and validation against setting both forms.
- `backend/internal/api`: optional catalog fields on the harness listing and its JSON contract; the workflow draft/source endpoints carry `skills`.
- `frontend/src`: harness wire types, catalog helper functions, shared model/thinking fields, a reusable multi-select control used by tools and skills, prompt-variable chips, and their tests.
- `README.md` and `AGENTS.md`: the optional catalog contract and the editor's fallback behavior.
