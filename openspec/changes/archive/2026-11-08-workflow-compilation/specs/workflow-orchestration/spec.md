# Spec Delta

## ADDED Requirements

### Requirement: Workflow definitions are compiled before a run is created

Before a run is created, the system SHALL compile the validated workflow into an executable plan. Compilation SHALL resolve every node's harness, model and reasoning effort, interactive and injection permissions, working-directory mode, timeout, absolute skill directories, and order. An omitted model identifier and the literal `default` SHALL stay omitted. Compilation SHALL fail run creation when a value cannot be resolved, naming every defect, and SHALL NOT validate model identifiers.

#### Scenario: Effective values are resolved once
- **WHEN** a node omits its harness and model while the workflow defaults set both
- **THEN** the plan carries the workflow's harness and model, and execution uses those values without consulting the defaults again

#### Scenario: Working directory mode is resolved
- **WHEN** a node declares no working directory
- **THEN** the plan records that it runs in the run workspace

#### Scenario: Default sentinel is not carried
- **WHEN** a node's model is the literal `default`
- **THEN** the plan carries no model identifier and the harness chooses its own model

#### Scenario: Resolution defects are reported together
- **WHEN** two nodes name skills that exist in none of the searched locations
- **THEN** run creation fails and the error names both nodes, their skills, and the searched locations

#### Scenario: Compilation failure has no run side effects
- **WHEN** compilation fails
- **THEN** no worktree, branch, or run record is created

#### Scenario: Unknown model is not a compile error
- **WHEN** a node names a model identifier that its harness does not support
- **THEN** compilation succeeds and the failure is reported when the node executes

## MODIFIED Requirements

### Requirement: Skill names resolve to absolute directories before execution

When a node names a skill, compilation SHALL resolve that name to one or more absolute directories and SHALL carry them in the plan. When no directory matches, the system SHALL fail run creation and SHALL name the skill and the locations searched.

#### Scenario: Skill resolves in the project
- **WHEN** a node names `openspec-explore` and that skill exists in the project's skill directory
- **THEN** the plan carries its absolute directory and the node's request names the skill

#### Scenario: Missing skill fails before start
- **WHEN** a node names a skill that exists in none of the searched locations
- **THEN** the run is not created and the failure names the skill and the searched locations

### Requirement: A failed run resumes at the failed node

A run that failed at a node SHALL be resumable at that node without re-executing any node that already completed. Before re-executing, resuming SHALL compile the workflow definition recorded with the run — not the file at its original path — and SHALL fail without executing a node when it no longer compiles. It SHALL restore the run's context, including the recorded change and checkpoints, from its persisted metadata.

#### Scenario: Resume skips completed nodes
- **WHEN** a run that failed at its third node is resumed
- **THEN** the first two nodes are not re-executed and execution continues at the third

#### Scenario: Resume preserves the resolved change
- **WHEN** a run resumes after its change was resolved
- **THEN** subsequent nodes receive the same change without re-deriving it

#### Scenario: Editing the workflow does not change a resumed run
- **WHEN** a workflow file is edited after a run is created and the run is then resumed
- **THEN** the run executes the definition recorded at creation, not the edited file

#### Scenario: Resume fails when the recorded definition no longer compiles
- **WHEN** a skill used by the recorded definition has since been removed
- **THEN** the resumed run fails before any node executes and names the missing skill and the searched locations
