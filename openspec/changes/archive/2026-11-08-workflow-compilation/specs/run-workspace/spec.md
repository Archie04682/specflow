# Spec Delta

## ADDED Requirements

### Requirement: A run retains the workflow definition it was created with

The system SHALL write the workflow source compiled for a run into the run's metadata directory before the run record is persisted, and SHALL record the copy's location in the run record. The copy SHALL remain with the run metadata and SHALL NOT change after creation.

#### Scenario: Definition is retained at creation
- **WHEN** a run is created
- **THEN** its metadata directory contains the workflow definition it was compiled from and the run record names it

#### Scenario: Retained definition is independent of its source
- **WHEN** the original workflow file is edited or removed after the run is created
- **THEN** the retained definition is unchanged

#### Scenario: Retained definition is committable metadata
- **WHEN** the user stages `.specflow/` in the project
- **THEN** the retained definition is included and the worktree contents are not
