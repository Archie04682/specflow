# multi-skill-nodes Specification

## Purpose

Lets a workflow node bind several skills so one agent step can follow more than one capability's instructions; every selected skill is resolved, loaded, and combined into the node prompt.

## Requirements

### Requirement: A node can bind several skills

A workflow node SHALL accept a `skills` list as well as the legacy singular
`skill` field. The effective selection SHALL be the plural list when present and
the singular value otherwise. A node that sets both SHALL be rejected as
invalid. The canonical file SHALL write the plural list when one is selected.

#### Scenario: Plural selection
- **WHEN** a node has `skills: [alpha, beta]`
- **THEN** both names are its effective skills

#### Scenario: Legacy selection
- **WHEN** a node has only `skill: alpha`
- **THEN** it compiles as a one-element skill list

#### Scenario: Both forms
- **WHEN** a node sets both `skill` and `skills`
- **THEN** validation fails with a problem naming the node

### Requirement: Every bound skill is resolved and propagated

Compilation SHALL resolve every effective skill to an absolute path in declared
order and SHALL fail creation when any skill is missing. The execution request
SHALL carry every resolved binding, while the first binding remains on the
legacy singular request field for older adapters.

#### Scenario: One missing skill
- **WHEN** one of several selected skills is not installed
- **THEN** run creation fails before a node starts

#### Scenario: Request propagation
- **WHEN** the engine executes a node with several skills
- **THEN** the request carries all resolved bindings in order

### Requirement: Pi combines multiple skills in the prompt

Pi SHALL load every selected skill path. With one skill it SHALL keep the
native `/skill:<name>` expansion. With several skills it SHALL inline each
skill's instructions in Pi's skill-block shape before the node prompt, because
Pi expands only one slash command. An unreadable selected skill SHALL fail as a
configuration failure before the process starts.

#### Scenario: Several skills
- **WHEN** the provider combines two skills
- **THEN** both skill blocks and the node prompt are in the user message

#### Scenario: One skill
- **WHEN** the node binds one skill
- **THEN** the prompt starts with `/skill:<name>` as before
