# run-workspace Specification

## Purpose

Gives every run its own disposable git workspace so an agent can work freely without disturbing the user's checkout, and leaves a per-node record of what changed that retries, diffs, and merges can all rely on.

## Requirements

### Requirement: Every run executes in its own worktree on its own branch

The system SHALL create a dedicated git worktree on a new branch for each run, checked out from the project's current commit, and SHALL execute the run's nodes there by default. The run SHALL record the source branch, the base commit, the branch name, and the worktree path in its metadata.

#### Scenario: Worktree is created from the starting commit
- **WHEN** a run is created for a project on branch `main` at commit `abc123`
- **THEN** a worktree exists on a new branch based on `abc123`, and the run metadata records `main`, `abc123`, the branch, and the worktree path

#### Scenario: The project checkout is not switched
- **WHEN** a run starts and its worktree is created
- **THEN** the project's own working tree remains on its original branch with its original contents

#### Scenario: Two runs do not share a workspace
- **WHEN** two runs are created for the same project
- **THEN** each has its own worktree and its own branch

### Requirement: A run refuses a project it cannot isolate safely

The system SHALL refuse to start a run when the project is not a git repository or its working tree has uncommitted changes, and SHALL explain which condition failed. It SHALL NOT stash, commit, or otherwise alter the user's working tree to proceed.

#### Scenario: Dirty working tree is refused
- **WHEN** a run is created for a project with modified or untracked files
- **THEN** the run is not created and the error names the uncommitted changes

#### Scenario: Non-git directory is refused
- **WHEN** a run is created for a directory that is not a git repository
- **THEN** the run is not created and the error says the project must be a git repository

### Requirement: Each node ends with a checkpoint commit

After a node finishes, the system SHALL record a checkpoint commit on the run branch capturing every change in the worktree, and SHALL record that commit's identifier against the node. A node that changed nothing SHALL record the previous checkpoint rather than creating an empty commit.

#### Scenario: Node changes are checkpointed
- **WHEN** a node completes after modifying files
- **THEN** a commit on the run branch contains those changes and its identifier is recorded for that node

#### Scenario: Node changes nothing
- **WHEN** a node completes without modifying the worktree
- **THEN** no commit is created and the node records the preceding checkpoint

### Requirement: A recorded checkpoint can be restored

The system SHALL be able to restore the worktree to any recorded checkpoint, discarding tracked changes and untracked files created after it, so a clean retry starts from the state the node first saw.

#### Scenario: Restore returns to the pre-node state
- **WHEN** the worktree is restored to a node's starting checkpoint
- **THEN** files match that checkpoint and files added after it are gone

#### Scenario: Restore does not remove ignored files
- **WHEN** the worktree is restored to a checkpoint
- **THEN** git-ignored files that are unrelated to the node's changes are left in place

### Requirement: Run metadata lives in the project under .specflow

The system SHALL store per-project run metadata under a `.specflow` directory in the project, keeping each run's record in its own directory, and SHALL create that directory when the first run starts.

#### Scenario: First run creates the metadata directory
- **WHEN** a project's first run is created
- **THEN** `.specflow/` exists in the project and contains that run's record

#### Scenario: Runs are kept apart
- **WHEN** a project has two runs
- **THEN** each run's metadata is stored in its own directory under `.specflow/`

### Requirement: Worktrees are never version-controlled while metadata may be

The system SHALL place worktrees under `.specflow/worktrees/` and SHALL ensure git ignores them, without requiring the user to edit the project's root `.gitignore`. Run metadata outside that directory SHALL remain committable so a user may keep it in version control if they choose.

#### Scenario: Worktree is ignored
- **WHEN** a run's worktree has been created
- **THEN** `git status` in the project does not report the worktree's contents as untracked

#### Scenario: Metadata can be committed
- **WHEN** the user stages `.specflow/` in the project
- **THEN** the run metadata is included and the worktree contents are not

### Requirement: Run metadata survives a restart

The system SHALL write each run's record at every state transition and SHALL write it atomically so a crash cannot leave a partially written record. Loading a persisted run SHALL reproduce its status, node results, resolved change, checkpoints, and workspace location.

#### Scenario: Run is loaded after a restart
- **WHEN** the server restarts while a run is in a terminal or retryable state
- **THEN** the run is listed with its status, node results, checkpoints, and resolved change intact

#### Scenario: Interrupted write does not corrupt the record
- **WHEN** the process stops while a run record is being written
- **THEN** the previously written record remains readable

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

### Requirement: Merging runs in the project checkout, never in the worktree

A merge of the run branch SHALL execute in the project's own checkout, not in the run worktree, because the worktree already has the run branch checked out. Before merging, the system SHALL refuse when the project has a branch other than the recorded source branch checked out or has uncommitted changes.

#### Scenario: Merge happens in the project checkout
- **WHEN** a merge node executes
- **THEN** it runs in the project root and the run worktree is unchanged by it

#### Scenario: Wrong branch is refused
- **WHEN** a merge node executes while the project has a branch other than the recorded source branch checked out
- **THEN** the merge is refused with an error naming the expected and the current branch

#### Scenario: Dirty project is refused
- **WHEN** a merge node executes while the project has uncommitted changes
- **THEN** the merge is refused before any branch is modified

### Requirement: Workspace lifetime follows the run's retryability

The system SHALL keep a run's worktree and branch while the run can still be retried or resumed, and SHALL remove the worktree once the run is completed or the user discards it. Removing a worktree SHALL NOT delete the run branch, so the merge and its history remain available.

#### Scenario: Completed run releases its worktree
- **WHEN** a run completes
- **THEN** its worktree is removed and its branch still exists

#### Scenario: Failed run keeps its worktree
- **WHEN** a run fails
- **THEN** its worktree is still present and the run can be retried or resumed

#### Scenario: Discarding removes the worktree but keeps the branch
- **WHEN** the user discards a failed run
- **THEN** the worktree is removed and the branch remains
