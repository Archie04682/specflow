# Spec Delta

## Purpose

Exposes the model, thinking-level, and tool choices an installed harness actually supports so the editor can offer real values instead of free text, without making catalog availability a requirement for listing or running the harness.

## ADDED Requirements

### Requirement: Harnesses may report an editor catalog

The harness listing SHALL include optional model, thinking-level, and tool
catalog fields when an adapter implements the catalog interface. A model entry
SHALL carry the workflow-facing id, a display name, its provider, whether it
supports reasoning, and the thinking levels it supports. The listing SHALL
carry the harness-wide thinking levels as a fallback. Tool entries SHALL carry a
name and short description. A catalog failure SHALL be reported as data and
SHALL NOT hide the harness entry.

#### Scenario: An adapter with a catalog
- **WHEN** the installed adapter implements the optional catalog
- **THEN** the harness entry carries its models, thinking levels, and tools

#### Scenario: A catalog cannot be read
- **WHEN** the adapter's catalog call fails
- **THEN** the harness entry remains listed with a catalog error and no catalog
  arrays

#### Scenario: An adapter without a catalog
- **WHEN** the installed adapter does not implement the optional interface
- **THEN** the harness entry is unchanged and the editor uses manual entry

### Requirement: The Pi catalog reflects local configuration

The Pi adapter SHALL build its catalog from the agent directory resolved by
`PI_CODING_AGENT_DIR` or `~/.pi/agent` without starting a runtime. It SHALL
honor enabled-model patterns, mark reasoning support, derive per-model thinking
levels from the model's level map with `off` always available, and return tool
names without exposing the internal ask tool. Missing or malformed files SHALL
produce an empty model list rather than an error.

#### Scenario: Enabled models only
- **WHEN** settings name enabled models
- **THEN** only those models are offered

#### Scenario: A non-reasoning model
- **WHEN** a model reports no reasoning support
- **THEN** its thinking levels contain only `off`

#### Scenario: No configuration
- **WHEN** the agent directory has no model files
- **THEN** the catalog still carries the built-in tools and thinking levels
