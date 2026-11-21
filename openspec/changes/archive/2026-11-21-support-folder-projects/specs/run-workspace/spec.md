## MODIFIED Requirements

### Requirement: Every run executes in its own worktree on its own branch

For a project whose workspace mode is git, the system SHALL create a dedicated
git worktree on a new branch for each run, checked out from the project's
current commit, and SHALL execute the run's nodes there by default. The run
SHALL record the source branch, the base commit, the branch name, and the
worktree path in its metadata. A folder project's workspace follows the folder
requirements instead.

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

For a git project, the system SHALL refuse to start a run when the project is
not a git repository or its working tree has uncommitted changes, and SHALL
explain which condition failed. It SHALL NOT stash, commit, or otherwise alter
the user's working tree to proceed. A folder project is not subject to either
check: the copy, not the working tree, is the run's base.

#### Scenario: Dirty working tree is refused
- **WHEN** a run is created for a project with modified or untracked files
- **THEN** the run is not created and the error names the uncommitted changes

#### Scenario: Non-git directory is refused
- **WHEN** a run is created for a directory that is not a git repository
- **THEN** the run is not created and the error says the project must be a git repository

#### Scenario: A folder project is not refused for uncommitted files
- **WHEN** a folder project's directory has modified or untracked files
- **THEN** the run is created and the files are copied as they are

### Requirement: Each node ends with a checkpoint commit

For a git project, after a node finishes, the system SHALL record a checkpoint
commit on the run branch capturing every change in the worktree, and SHALL
record that commit's identifier against the node. A node that changed nothing
SHALL record the previous checkpoint rather than creating an empty commit. A
folder run records a snapshot instead.

#### Scenario: Node changes are checkpointed
- **WHEN** a node completes after modifying files
- **THEN** a commit on the run branch contains those changes and its identifier is recorded for that node

#### Scenario: Node changes nothing
- **WHEN** a node completes without modifying the worktree
- **THEN** no commit is created and the node records the preceding checkpoint

### Requirement: A recorded checkpoint can be restored

For a git project, the system SHALL be able to restore the worktree to any
recorded checkpoint, discarding tracked changes and untracked files created
after it, so a clean retry starts from the state the node first saw. A folder
run restores its copy from a snapshot instead.

#### Scenario: Restore returns to the pre-node state
- **WHEN** the worktree is restored to a node's starting checkpoint
- **THEN** files match that checkpoint and files added after it are gone

#### Scenario: Restore does not remove ignored files
- **WHEN** the worktree is restored to a checkpoint
- **THEN** git-ignored files that are unrelated to the node's changes are left in place

### Requirement: Worktrees are never version-controlled while metadata may be

For a git project, the system SHALL place worktrees under
`.specflow/worktrees/` and SHALL ensure git ignores them, without requiring the
user to edit the project's root `.gitignore`. Run metadata outside that
directory SHALL remain committable so a user may keep it in version control if
they choose. A folder project's copy lives in the same directory, which no
repository controls.

#### Scenario: Worktree is ignored
- **WHEN** a run's worktree has been created
- **THEN** `git status` in the project does not report the worktree's contents as untracked

#### Scenario: Metadata can be committed
- **WHEN** the user stages `.specflow/` in the project
- **THEN** the run metadata is included and the worktree contents are not

### Requirement: Merging runs in the project checkout, never in the worktree

For a git project, a merge of the run branch SHALL execute in the project's own
checkout, not in the run worktree, because the worktree already has the run
branch checked out. Before merging, the system SHALL refuse when the project
has a branch other than the recorded source branch checked out or has
uncommitted changes. A folder project has no merge: applying its run writes the
files back.

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

For a git project, the system SHALL keep a run's worktree and branch while the
run can still be retried or resumed, and SHALL remove the worktree once the run
is completed or the user discards it. Removing a worktree SHALL NOT delete the
run branch, so the merge and its history remain available. A folder run keeps
its copy until the run is applied or discarded.

#### Scenario: Completed run releases its worktree
- **WHEN** a run completes
- **THEN** its worktree is removed and its branch still exists

#### Scenario: Failed run keeps its worktree
- **WHEN** a run fails
- **THEN** its worktree is still present and the run can be retried or resumed

#### Scenario: Discarding removes the worktree but keeps the branch
- **WHEN** the user discards a failed run
- **THEN** the worktree is removed and the branch remains

## ADDED Requirements

### Requirement: A project chooses a git or folder workspace

The system SHALL read a project's workspace mode from `.specflow/config.yaml`,
defaulting to git when the file does not set one, and SHALL record the mode in
every run it creates. The mode SHALL be changeable only while the project has
no runs, and a change SHALL affect runs created after it. A repository MAY use
folder mode: the mode, not the presence of `.git`, selects the workspace.

#### Scenario: An unset mode reads as git
- **WHEN** the project's config does not name a mode
- **THEN** the project reports git and runs use a worktree

#### Scenario: The mode is recorded on the run
- **WHEN** a run is created
- **THEN** its record names the mode it ran under

#### Scenario: A mode change after the first run is refused
- **WHEN** the project has at least one run
- **THEN** changing the mode is refused with that reason and the recorded mode is unchanged

### Requirement: A folder project's run works on a copy of the project

The system SHALL, when a folder project's run is created, copy the project's
files into `.specflow/worktrees/<id>` excluding `.specflow/` and the repository's
git metadata, execute the run's nodes in that copy, and record a manifest of the
copied files as the run's base. The project's own files SHALL remain unchanged
until the run is applied.

#### Scenario: The project is untouched while the run works
- **WHEN** a folder run executes a node that rewrites files
- **THEN** the project's files are unchanged and the rewritten files are in the run's copy

#### Scenario: The copy excludes the run metadata
- **WHEN** a folder run's copy is created
- **THEN** it does not contain `.specflow/`, and the run's record, snapshot, and definition stay in the project

#### Scenario: The base manifest records the copied files
- **WHEN** a folder run is created
- **THEN** the run can later tell which files it added, changed, and deleted against that manifest

#### Scenario: Git metadata is never copied
- **WHEN** a folder run's project is also a git repository
- **THEN** neither the copy nor its manifest contains `.git`, so applying the run cannot write git objects into the project

#### Scenario: A project-directory node is refused
- **WHEN** a folder run's flow declares a node that runs in the project directory
- **THEN** the run is not created and the error names the node

### Requirement: A folder run snapshots the copy after each node

The system SHALL record a snapshot of the run's copy after a node finishes,
SHALL record that snapshot against the node, and SHALL be able to restore the
copy to any recorded snapshot, discarding files created after it, so a clean
retry starts from the state the node first saw.

#### Scenario: A node's snapshot is recorded
- **WHEN** a node completes
- **THEN** a snapshot of the copy exists and its identifier is recorded for that node

#### Scenario: Restore returns to the pre-node state
- **WHEN** the copy is restored to a node's starting snapshot
- **THEN** files match that snapshot and files added after it are gone

### Requirement: One folder run at a time per project

The system SHALL refuse to create a folder run while the same project still
holds a copy: a run that is running, failed, or completed and not yet applied or
discarded. The refusal SHALL name the run holding the project. Applying or
discarding that run SHALL release the project.

#### Scenario: A second folder run is refused
- **WHEN** a folder run is created while another folder run for the project is unfinished or awaiting apply
- **THEN** the run is not created and the refusal names the run holding the project

#### Scenario: Applying or discarding releases the project
- **WHEN** the held run is applied or discarded
- **THEN** a new folder run can be created

### Requirement: A completed folder run is applied or discarded

The system SHALL keep a completed folder run's copy until the operator applies
or discards it, SHALL record which happened, and SHALL NOT stage, commit, or
add anything to history. Applying SHALL write the run's added and changed files
into the project, delete the files the run deleted, report both, and refuse
when a project file no longer matches the base manifest. Discarding SHALL
remove the copy without touching the project. Both SHALL remove the run's
snapshots and keep its metadata.

#### Scenario: Apply writes the run's changes
- **WHEN** a completed folder run is applied
- **THEN** its added and changed files are in the project, its deleted files are gone, and nothing was committed

#### Scenario: Apply reports what it wrote and deleted
- **WHEN** a completed folder run is applied
- **THEN** the response names the files it wrote and the files it deleted

#### Scenario: Apply refuses a project that changed
- **WHEN** a project file no longer matches the base manifest when the run is applied
- **THEN** the apply is refused, the paths are named, and the project is unchanged

#### Scenario: Discard leaves the project untouched
- **WHEN** a completed folder run is discarded
- **THEN** the copy and snapshots are gone and the project's files are unchanged

#### Scenario: The run records the outcome
- **WHEN** a folder run is applied or discarded
- **THEN** the run reports which of the two happened
