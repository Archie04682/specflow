# Spec Delta

## MODIFIED Requirements

### Requirement: A project shares skill directories across harnesses

The system SHALL resolve a node's skill from the first configured root that contains it, defaulting to `.specflow/skills`, `.agents/skills`, `.dsh/skills`, `.pi/skills`, `~/.dsh/skills`, and `~/.agents/skills` in that order. A non-empty `skills` list in `.specflow/config.yaml` SHALL replace that list. The resolved absolute directory SHALL be passed to the harness, and no harness-specific copy SHALL be required.

#### Scenario: A shared skill is bound by path

- **WHEN** a skill directory exists under `.specflow/skills` and a node names it
- **THEN** the plan resolves that directory and the harness receives its absolute path

#### Scenario: The shared root wins a duplicate

- **WHEN** the same directory name exists under `.specflow/skills` and `.dsh/skills`
- **THEN** the `.specflow/skills` copy is resolved

#### Scenario: An installed skill is resolvable

- **WHEN** a skill directory exists under `.agents/skills` and a node names it
- **THEN** the plan resolves that directory and the harness receives it without a copy

#### Scenario: A project's own skill beats the shared install

- **WHEN** the same directory name exists under `.specflow/skills` and `.agents/skills`
- **THEN** the `.specflow/skills` copy is resolved

#### Scenario: Configured roots replace the defaults

- **WHEN** `.specflow/config.yaml` sets `skills`
- **THEN** exactly those roots are searched, in the listed order
