# Design

## D1 — Shared roots with explicit precedence

Skill roots are ordered by precedence and resolved relative to the project root
(`~/` expands to the home directory). The defaults become:

1. `.specflow/skills` — shared, project-versioned, harness-agnostic
2. `.dsh/skills` — existing DSH convention
3. `.pi/skills` — existing Pi project convention
4. `~/.dsh/skills` — existing DSH global convention

A non-empty `skills` list in `.specflow/config.yaml` replaces the defaults
entirely, in the listed order; this keeps the existing config contract. The
shared root is first so one copy of a skill wins for every harness. The resolver
stays `ResolveSkill`: first directory with the name, no merging.

## D2 — Catalog scanning is best effort

`ListSkills` reads each root, keeps directories that contain a `SKILL.md`, and
returns them in root order, alphabetically within a root. The first occurrence of
a directory name wins, matching resolution precedence. A `SKILL.md` frontmatter
`description` is extracted with the YAML parser; malformed or absent frontmatter
only leaves the description empty. Missing or unreadable roots are skipped so a
broken install cannot hide the rest of the catalog. A directory without
`SKILL.md` is not listed; the catalog is for discoverable skills, not arbitrary
directories.

## D3 — API shape and safety

`GET /api/skills?project=` opens the project, reads its skill sources, and
returns `{"skills": [{"name", "description", "source"}]}`. `source` is the
configured root label, never an absolute host path. An unusable project is `400`
(the path is client state, like the workflow catalog); a project with no skills
returns an empty array. The endpoint is read-only and does not load or validate
the skill content.

## D4 — Editor behavior

The editor fetches the catalog once on mount and passes it to the node
inspector. With a catalog, Skill is a native grouped `<select>`:

- `No skill` clears the value.
- Installed skills are grouped by `source`.
- A current value absent from the catalog is preserved as an option marked
  `(not installed)`; the hint warns that a run will reject it.
- The selected entry's source and frontmatter description are shown below the
  field (clamped, full text in `title`).

The field is optional and only shown for agent nodes. If the catalog request
fails, or is still loading, the field remains the existing text input with an
`aria-describedby` hint; the editor never blocks on the catalog. Selecting writes
the directory name into the draft, which is exactly what `ResolveSkill` consumes.

## D5 — Binding needs no harness copy

Pi is already started with `--no-skills` plus one `--skill <absolute path>` per
bound skill, and the prompt is prefixed with `/skill:<name>`. Ambient discovery
from `.pi/skills`, `.dsh/skills`, or `.claude/skills` is deliberately disabled
for a node, so the folder location is irrelevant as long as the resolver finds it
and the adapter can read it. `harness-pi-adapter` already specifies this
deterministic binding; this change only changes what is discoverable and which
directory is resolved first. Other harnesses that accept an explicit skill path
get the same benefit; no adapter change is implied.

## D6 — Tests

- Workspace: default order includes `.specflow/skills` first; a config list
  replaces defaults; labels and `~` expansion.
- Catalog: frontmatter description, root order, alphabetical order, duplicate
  shadowing, missing roots, broken entries, empty non-nil result.
- API: 200 shape, precedence, empty array, missing/unusable project 400.
- Frontend: catalog fetch, grouped dropdown selection, uninstalled value
  preservation, fallback input on catalog failure; editor page still passes its
  existing suite.

## Risks / Trade-offs

| Risk | Mitigation |
| :--- | :--- |
| A user sets `skills` in config and loses the shared default | Documented: include `.specflow/skills` in the list; the default config template lists it. |
| A skill in a higher root shadows a lower one silently | The catalog shows the winning `source`; resolution already used first-match precedence. |
| Untrusted frontmatter description is rendered | React escapes it as text; it is clamped and never executed. |
| Catalog is stale after adding a skill while the editor is open | Reload the editor/start screen; no file watcher in this change. |
| A directory without `SKILL.md` is resolved by name but not listed | The catalog is for valid skill bundles; a run-time load still reports the real failure. |

## Open Questions

None. Creating/editing skill contents and a registry are explicit non-goals.
