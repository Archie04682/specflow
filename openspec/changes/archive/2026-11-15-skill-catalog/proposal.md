# Proposal

## Why

The editor's Skill field is a free-text input. Installed skills are invisible, names are easy to mistype, and a project cannot tell which skills are available without reading directories by hand. The usual assumption is that a skill must live in a harness-specific folder (`.pi/skills`, `.dsh/skills`, `.claude/skills`), which would duplicate one skill once per harness.

That assumption is wrong for this architecture. When a run is created, SpecFlow resolves the skill name to an absolute directory and, for Pi, starts it with `--no-skills --skill <absolute path>` and prompts `/skill:<name>`. Pi never depends on ambient discovery. A single shared folder is therefore enough, and the missing piece is discoverability in the editor.

## What Changes

- Make `<project>/.specflow/skills` the **first default skill root**, ahead of `.dsh/skills`, `.pi/skills`, and `~/.dsh/skills`. `.specflow/config.yaml`'s `skills` list still overrides the defaults, so `.claude/skills` or any other path can be added without copies.
- Add `GET /api/skills?project=` returning installed skills as `{name, description, source}`: directory name, `SKILL.md` frontmatter description, and the configured source label. First root wins for duplicate names; unreadable roots/entries are skipped.
- Replace the node inspector's Skill input with a dropdown grouped by source root when the catalog is available. A node's existing but uninstalled skill stays visible and is marked; on catalog failure the field falls back to a text input with an inline hint.
- Document that no harness-specific copy is needed: a skill installed once under `.specflow/skills` is passed to the harness by absolute path.
- No adapter change is needed: Pi's deterministic path binding and `/skill:<name>` invocation already exist.

**Not included:** creating or editing skill contents from the UI, installing skills from a registry, skill versioning, or copying skills into harness folders.

## Capabilities

### New Capabilities

- `skill-catalog`: discovering installed skills from shared and configured roots, exposing them to the editor, and binding the selected skill by absolute path.

### Modified Capabilities

None.

## Impact

- `backend/internal/workspace`: default roots gain `.specflow/skills`; skill sources carry labels.
- `backend/internal/openspec`: catalog scanning and frontmatter descriptions.
- `backend/internal/orchestrator` + `backend/internal/api`: read-only `GET /api/skills`; no run or persistence changes.
- `frontend/src`: skill catalog client, editor fetch, grouped dropdown with unknown-value preservation and text fallback.
- `README.md` and `AGENTS.md`: shared-root decision and usage.
