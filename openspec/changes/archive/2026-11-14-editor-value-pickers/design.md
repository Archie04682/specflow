# Design

## Context

See `proposal.md` for motivation and the two capability specs for the contract.
The relevant existing pieces:

- `harness.Adapter` is deliberately small: descriptor, capabilities, probe,
  run. Every adapter must implement all four, so it is not the place for an
  editor-only concern.
- The harness listing already probes each adapter and returns a JSON entry.
  The editor already calls typed API modules and has a skill catalog with a
  fallback pattern worth mirroring.
- Pi's agent directory holds `settings.json`, `models-store.json`, and an
  optional `models.json`. The store records each provider's models, including
  reasoning support and a `thinkingLevelMap` whose non-null values are the
  levels that model accepts.
- `RenderPrompt` substitutes a closed set from `RunContext.Values`:
  `objective`, `change`, `run_id`, `workdir`. Anything else fails before the
  node starts.
- A workflow node used to bind one `skill`, compiled to one absolute path. Pi
  loads skills with `--no-skills --skill <path>` and only expands a single
  `/skill:<name>` command at the start of a prompt.

## Goals / Non-Goals

**Goals:**

- Make the values the editor offers come from the same place execution does.
- Keep every existing adapter and every workflow unchanged.
- Keep manual entry when a catalog is unavailable; never block editing.

**Non-Goals:**

- Runtime/CLI model discovery, provider login flows, or remote model catalogs.
- A tool-capability contract beyond names and descriptions.
- Changing prompt rendering, saving, or compile behavior.

## Decisions

### D1 — The catalog is an optional interface, not a wider Adapter

`CatalogProvider` is a separate interface in `harness`:

```go
type CatalogProvider interface {
    Catalog(ctx context.Context) (Catalog, error)
}
```

`Registry`, the API, and tests type-assert it. An adapter that only executes
work keeps compiling and keeps the run contract untouched. The API treats a
catalog error as data (`catalogError`) rather than failing the harness listing.

### D2 — Pi reads its configuration; it does not start a runtime

`pi.Adapter.Catalog` reads the agent directory resolved from
`PI_CODING_AGENT_DIR` or `~/.pi/agent`:

- `settings.json` for `enabledModels`, `defaultModel`, `defaultProvider`, and
  `defaultTools`;
- `models-store.json` for the provider/model snapshots;
- optional `models.json` for locally declared providers, merged and deduped.

This is cheap enough to call while rendering the editor, has no account or
network side effects, and uses the same metadata the runtime will use. The
listing caps the call with the existing harness probe timeout.

Model IDs are `provider/id`, matching the value written to a workflow. The
enabled-model patterns support `*`; when a model's `thinkingLevelMap` names a
level with a non-null value, that level is offered, with `off` always present.
A non-reasoning model offers only `off`. Pi's built-in tools are returned with
short descriptions; the internal ask tool is not selectable.

### D3 — One shared model/thinking component for nodes and defaults

Both the node inspector and the workflow defaults panel render the same
`HarnessModelFields`. It resolves the harness in play, shows a grouped model
select, and derives thinking levels from the selected model. The rules are:

- `""` means inherit: the workflow default for a node, the harness default for
  the defaults panel; the explicit `default` sentinel stays available.
- A current value that is absent from the catalog stays visible as
  `(not in catalog)` / `(unsupported)` instead of silently disappearing.
- Changing the model drops a thinking level the new model does not support.
- Thinking is disabled, with an explanation, when the model or harness reports
  no reasoning support.
- Without a catalog the fields are the previous text inputs with a hint that
  says why.

### D4 — Tools and skills are multi-select dropdowns

Both controls use one reusable `MultiSelectField`: a closed trigger that shows
selected values as chips, a panel of checkbox rows (grouped when the values have
sources), an optional custom-name entry, and a clear action. An empty tool set
writes `tools: undefined`, preserving the "use the harness default" semantics.
Selected tool names outside the catalog remain visible and removable. Skills
are grouped by source and preserve an uninstalled selected name. Adapters
without catalogs keep the comma-separated text fields.

The panel is an inline disclosure rather than a portal: it participates in the
editor's scroll container, so it cannot be clipped by the side panel, and it
closes on the trigger, outside pointer-down, or Escape.

### D5 — Multiple skills are a list, and Pi combines their instructions

The workflow gains `skills []string`; the old `skill string` remains readable
and compiles as a one-element list. A node that sets both is invalid, so there
is one source of truth. The draft round-trips the list and the canonical writer
emits `skills:`.

Compilation resolves every selected skill to an absolute path and carries all
bindings on the request (`Request.Skills`, with the first also kept on the
legacy `Request.Skill` field for old adapters). Pi loads every path. With one
skill it keeps the existing `/skill:<name>` prompt expansion. With several it
inlines each `SKILL.md` body in Pi's own `<skill name=… location=…>` block shape
before the node prompt, because Pi only expands the first slash command; this
is the same content Pi would inject, just composed by the adapter. A missing or
unreadable selected skill fails configuration before the process starts.

### D6 — Prompt variables mirror the closed server set

The chip list is a single constant in `frontend/src/editor/promptVariables.ts`.
Clicking a chip inserts `{{token}}` at the textarea's caret and keeps focus.
The hint says an unknown placeholder fails compilation; this is the same
contract as `RenderPrompt`, not a new one.

## Risks

- **Config schema drift.** Pi's `models-store.json` is an implementation detail.
  The reader is tolerant: unknown fields are ignored, missing files produce an
  empty model list, and the editor falls back to manual entry.
- **Large catalogs.** Providers can expose many models; the select is grouped by
  provider and the catalog call is not on the run path.
- **Stale catalog.** It is read at editor open; a model added later needs a
  reload. Manual entry remains available for adapters whose catalog is stale.
- **Tool names are not capabilities.** The picker does not validate that a tool
  will exist at run time; the run's preflight remains authoritative.
- **Multi-skill changes the prompt.** Combining skill instructions grows the
  node prompt and may duplicate guidance across skills. It is the operator's
  selection; compilation still fails early for a missing skill, and the single
  skill path is unchanged.
- **Old adapters.** `Request.Skills` is additive and the first binding remains
  on `Request.Skill`; an adapter that only reads the singular field still runs
  with the first selected skill.
