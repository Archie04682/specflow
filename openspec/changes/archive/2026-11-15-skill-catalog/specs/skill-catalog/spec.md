## ADDED Requirements

### Requirement: A project shares skill directories across harnesses

The system SHALL resolve a node's skill from the first configured root that contains it, defaulting to `.specflow/skills`, `.dsh/skills`, `.pi/skills`, and `~/.dsh/skills` in that order. A non-empty `skills` list in `.specflow/config.yaml` SHALL replace that list. The resolved absolute directory SHALL be passed to the harness, and no harness-specific copy SHALL be required.

#### Scenario: A shared skill is bound by path

- **WHEN** a skill directory exists under `.specflow/skills` and a node names it
- **THEN** the plan resolves that directory and the harness receives its absolute path

#### Scenario: The shared root wins a duplicate

- **WHEN** the same directory name exists under `.specflow/skills` and `.dsh/skills`
- **THEN** the `.specflow/skills` copy is resolved

#### Scenario: Configured roots replace the defaults

- **WHEN** `.specflow/config.yaml` sets `skills`
- **THEN** exactly those roots are searched, in the listed order

### Requirement: Installed skills are discoverable

`GET /api/skills?project=` SHALL return installed skills as `{name, description, source}` entries from the configured roots, keeping only the first occurrence of each name and skipping unreadable roots or entries. A missing or unusable project SHALL return `400`; a project with no skills SHALL return `{"skills": []}`.

#### Scenario: The catalog lists a shared skill

- **WHEN** a project has a skill directory with `SKILL.md` under a configured root
- **THEN** the response names it with its frontmatter description and source label

#### Scenario: Duplicate names are not duplicated

- **WHEN** two roots provide the same skill directory name
- **THEN** only the higher-precedence entry is returned

#### Scenario: One broken entry does not hide the catalog

- **WHEN** a root or a `SKILL.md` cannot be read
- **THEN** the other installed skills are still returned

### Requirement: The editor offers installed skills

When the node inspector has a skill catalog, the Skill field SHALL be a dropdown grouped by source root, SHALL preserve and mark a current skill that is not installed, and SHALL write the selected name into the draft. If the catalog cannot be loaded, the field SHALL remain a free-text input with an inline explanation.

#### Scenario: Selecting an installed skill

- **WHEN** the operator selects `ui-ux-pro-max` from the dropdown
- **THEN** the draft node's skill becomes `ui-ux-pro-max`

#### Scenario: An uninstalled value is preserved

- **WHEN** the node already names a skill that the catalog does not list
- **THEN** the field keeps the value visible and marks it as not installed

#### Scenario: Catalog failure keeps the field usable

- **WHEN** the catalog request fails
- **THEN** the field remains a free-text input with an inline hint

### Requirement: Skill selection is bound by path, not by folder convention

A skill installed once under `.specflow/skills` SHALL be usable by a harness that accepts an explicit skill path. The editor and plan SHALL NOT copy a skill into `.pi/skills`, `.dsh/skills`, or `.claude/skills` to make it available.

#### Scenario: Pi receives the shared skill without a copy

- **WHEN** a Pi node binds a skill that resolves under `.specflow/skills`
- **THEN** Pi is started with that absolute path and no harness-specific copy exists
