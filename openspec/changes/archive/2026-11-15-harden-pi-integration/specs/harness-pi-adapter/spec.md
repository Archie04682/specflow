# Spec Delta

## MODIFIED Requirements

### Requirement: Deterministic skill binding

When a node binds a skill, the adapter SHALL load that skill from an absolute path and SHALL invoke it explicitly by name, so the execution does not depend on skills discovered from the user's global environment. When the skill's path does not exist, the adapter SHALL fail before submitting any prompt. After Pi starts, the adapter SHALL confirm the skill loaded and SHALL fail before prompting when it did not.

#### Scenario: Bound skill is invoked explicitly
- **WHEN** a node binds the `openspec-explore` skill and its directory exists
- **THEN** Pi is started with that skill loaded and the prompt names the skill explicitly

#### Scenario: Missing skill path fails before prompting
- **WHEN** a node binds a skill whose path does not exist
- **THEN** the execution fails as a configuration failure and no prompt is sent

#### Scenario: Unloadable skill fails before prompting
- **WHEN** a node binds a skill directory that Pi accepts as a path but does not load, such as a `SKILL.md` without a description
- **THEN** the execution fails as a configuration failure naming the skill, before any prompt is sent

## ADDED Requirements

### Requirement: The bridge handshake is not telemetry

The adapter SHALL consume the embedded extension's bridge-ready notification as a startup handshake and SHALL NOT publish it as a node event.

#### Scenario: Bridge marker is swallowed during an execution
- **WHEN** the bridge extension loads at the start of a node execution
- **THEN** no node event carries the bridge-ready message
