# Tasks

## 1. Shared roots

- [x] 1.1 Add `.specflow/skills` as the first default skill root, keep the existing harness roots after it, expose labeled `SkillSource`s, and update the config template comment; verify workspace tests for the default order, config override, and `~` expansion.

## 2. Catalog

- [x] 2.1 Add `ListSkills`: scan roots in precedence order, require `SKILL.md`, read the frontmatter description, sort within a root, shadow duplicate names from lower roots, and skip unreadable roots/entries; verify tests for frontmatter, ordering, duplicates, missing roots, broken entries, and an empty non-nil result.

## 3. API

- [x] 3.1 Add `ReadSkillCatalog` and `GET /api/skills?project=`, returning `{name, description, source}` entries and an empty array for no skills; verify `httptest` coverage for 200/precedence, empty project, missing project, and unusable project.

## 4. Editor

- [x] 4.1 Add the skill catalog client and fetch it when the flow editor mounts; pass it to the node inspector.
- [x] 4.2 Replace the Skill input with a source-grouped dropdown when the catalog is available, preserve and mark uninstalled values, show the selected source/description, and retain the text-input fallback with an inline hint on failure; verify NodeInspector and WorkflowEditor tests.
- [x] 4.3 Document the shared folder, the dropdown, and the explicit path binding in README and AGENTS.

## 5. Verification

- [x] 5.1 Run `make test`, `make build`, and `openspec validate --changes --strict`.
