## MODIFIED Requirements

### Requirement: A project's readiness is reported without changing it

Readiness SHALL report for one project: its workspace mode, whether it was
recorded, and whether it can still change, its git state, whether `.specflow/`
exists, its flows with their skills and which do not resolve, the `openspec`
CLI and harness availability, offline mode, and which uncommitted paths look
like secrets. Only a git project SHALL count the git state as a gap. The read
SHALL change nothing; a path that is not a directory SHALL be reported as `400`
naming the reason.

#### Scenario: A fresh folder reports every gap

- **WHEN** a registered directory is a clean git repository with no flow and no skills
- **THEN** the report names the missing flow, the skills the flows would need, and the absent `.specflow/` directory, and creates none of them

#### Scenario: A path that is not a repository is a gap, not an error

- **WHEN** the directory exists but is not a git repository
- **THEN** the report returns `200` with the repository check failed rather than failing the read

#### Scenario: A missing directory is refused

- **WHEN** the path does not exist or is not a directory
- **THEN** the response is `400` with an error naming the path

#### Scenario: An empty repository is a gap

- **WHEN** the directory is an initialised repository with no commit
- **THEN** the report returns `200` with the repository and branch present, the commit absent, and the project not ready

#### Scenario: Reading twice changes nothing

- **WHEN** readiness is read twice
- **THEN** the project tree, its git status, and the report are unchanged between the reads

#### Scenario: A folder project has no git gap

- **WHEN** the project's workspace mode is folder and the directory is not a repository
- **THEN** the report names the mode, states the missing repository without counting it as a gap, and can still report the project ready

### Requirement: A repository is initialised on request

The system SHALL initialise a git repository in a project directory on request,
naming the initial branch `main` where git supports it and falling back where it
does not. It SHALL refuse a directory that already belongs to a repository,
including one nested inside another, SHALL refuse a project whose workspace
mode is folder, naming the mode, and SHALL NOT create a commit or change any
file in the project.

#### Scenario: A plain folder becomes a repository

- **WHEN** a directory that is not a repository is initialised
- **THEN** `.git` exists, the branch is `main`, the project's files are untouched, and no commit was made

#### Scenario: An existing repository is refused

- **WHEN** the directory already belongs to a repository
- **THEN** the request is refused naming the repository root and nothing changes

#### Scenario: A nested directory is refused

- **WHEN** the directory sits inside another repository
- **THEN** the request is refused naming that repository's root rather than creating a nested one

#### Scenario: A folder project is refused

- **WHEN** the project's workspace mode is folder
- **THEN** the request is refused naming the mode and no repository is created

### Requirement: The default flow is seeded from the executable's template

The system SHALL seed a project's flow by writing an embedded template to
`.specflow/workflows/<reference>.yaml`, creating the directory when needed, and
SHALL refuse to overwrite an existing file, reporting the refusal without
changing it. For a folder project it SHALL write the template that has no merge
node.

#### Scenario: Seeding an empty project

- **WHEN** a project has no `.specflow/workflows/<reference>.yaml`
- **THEN** the file is written from the template and the catalog lists it as a valid flow

#### Scenario: Seeding twice is refused

- **WHEN** the flow file already exists
- **THEN** the response reports the conflict, the existing file is byte-for-byte unchanged, and no second file is created

#### Scenario: A project copy keeps its edits

- **WHEN** the operator has edited the project's copy of the default flow
- **THEN** seeding again changes nothing and says why

#### Scenario: A folder project gets the merge-free flow

- **WHEN** the project's workspace mode is folder and the default flow is seeded
- **THEN** the written flow contains no merge node

### Requirement: Uncommitted provisioning is reported rather than committed

The system SHALL report the paths a provisioning action left uncommitted and
state that starting a run requires them committed. Provisioning SHALL NOT
create a commit, stage a path, or change an ignore rule on the project's
behalf. On request it SHALL commit every uncommitted path, or append named
paths to `.gitignore`; a commit SHALL refuse paths that look like secrets and
name them, and SHALL refuse a clean tree.

#### Scenario: Installed skills are uncommitted

- **WHEN** an install writes `.agents/skills/`
- **THEN** readiness lists that path as uncommitted and names committing it as the remaining step before a run

#### Scenario: A seeded flow does not block a run

- **WHEN** a seed writes `.specflow/workflows/<reference>.yaml` and nothing else is uncommitted
- **THEN** readiness reports no run-blocking path, because the run gate ignores `.specflow/`

#### Scenario: A committed project reports no such gap

- **WHEN** the operator commits the provisioned paths
- **THEN** readiness no longer reports them and the project is ready

#### Scenario: The app never commits

- **WHEN** a provisioning action runs without a commit request
- **THEN** the project's history, index, and ignore files are unchanged

#### Scenario: The operator commits

- **WHEN** the operator asks to commit while paths are uncommitted
- **THEN** the paths are committed together and readiness no longer reports them

#### Scenario: Secrets are refused

- **WHEN** `.env` is uncommitted and the operator asks to commit
- **THEN** the commit is refused naming `.env`, and nothing is staged or committed

#### Scenario: Ignoring unblocks the commit

- **WHEN** the refused path is appended to `.gitignore` on request
- **THEN** readiness stops reporting it and a later commit succeeds

#### Scenario: A clean tree is refused

- **WHEN** the operator asks to commit with nothing uncommitted
- **THEN** the request is refused with that reason and no commit is created

## ADDED Requirements

### Requirement: A project's workspace mode is chosen at setup

The system SHALL record a project's workspace mode on request, accepting git or
folder, writing it to `.specflow/config.yaml` without discarding the file's
other settings. Choosing git SHALL initialise a repository when the directory is
not one. It SHALL refuse an unknown mode, and SHALL refuse a change while the
project has runs, naming why. It SHALL NOT seed a flow, install a skill, or
start a run.

#### Scenario: Choosing folder writes the mode and creates no repository

- **WHEN** the operator chooses folder for a directory that is not a repository
- **THEN** the config records folder, no `.git` is created, and the project's files are unchanged

#### Scenario: Choosing git initialises a repository

- **WHEN** the operator chooses git for a directory that is not a repository
- **THEN** the config records git and a repository exists on the initial branch with no commit

#### Scenario: An unknown mode is refused

- **WHEN** the request names a mode other than git or folder
- **THEN** the request is refused and the config is unchanged

#### Scenario: A change after the first run is refused

- **WHEN** the project has at least one run
- **THEN** changing the mode is refused with that reason and the recorded mode is unchanged

#### Scenario: Setup does not provision

- **WHEN** the mode is recorded
- **THEN** no flow is seeded, no skill is installed, and `.specflow/` holds only the config
