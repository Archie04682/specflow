# project-onboarding Specification

## Purpose

Turns a registered folder into a project that can start a run: it reports what is missing, lets the operator choose a git or folder workspace, seeds the default flow for that mode, and installs the skills a flow binds from an online or an offline source — without changing the project unasked.

## Requirements

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
### Requirement: A flow's required skills are derived from the flow

For each flow the report SHALL derive the required skill names from that definition's node skill bindings, including the older singular `skill` field, and SHALL report the names that resolve and the names that do not, listing the searched roots once for the project. A definition that cannot be parsed SHALL be reported with its problems instead of being dropped from the report.

#### Scenario: Missing skills are named per flow

- **WHEN** a flow binds four skills and two are installed
- **THEN** the report names the two missing skills under that flow and lists the roots that were searched

#### Scenario: The singular field counts

- **WHEN** a node binds a skill through the older singular `skill` field
- **THEN** that name appears in the flow's required skills

#### Scenario: A broken definition does not hide the others

- **WHEN** one workflow file cannot be parsed
- **THEN** that flow is listed invalid with its problems and the other flows are still resolved

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
### Requirement: Skills install from a directory or an archive

The system SHALL install skills from a configured source that is a directory, a `.zip`, a `.tar`, a `.tar.gz`, or a `.tgz`, SHALL discover skills by locating directories that contain a `SKILL.md`, and SHALL write each to `.agents/skills/<name>` using the containing directory's name. A local source SHALL NOT require Node.js, npm, or a network.

#### Scenario: A directory of skills

- **WHEN** the source directory contains three directories that each hold a `SKILL.md`
- **THEN** three skills are written under `.agents/skills/` and the report names them

#### Scenario: A GitHub download is located anyway

- **WHEN** the archive nests the skills under a single top-level directory such as `OpenSpec-1.16.1/skills/`
- **THEN** the skills are still discovered and installed

#### Scenario: One skill directory is a valid source

- **WHEN** the source is a single directory containing `SKILL.md`
- **THEN** that one skill is installed

#### Scenario: A source with no skills is refused

- **WHEN** the source contains no directory with a `SKILL.md`
- **THEN** the response is refused naming the source and nothing is written

### Requirement: Extraction is confined and bounded

Extraction SHALL write only directories that contain a `SKILL.md`, SHALL reject an entry whose path is absolute or resolves outside the extraction root, SHALL NOT create symlinks or hardlinks, and SHALL refuse an archive with more than 1000 entries or more than 25 MiB of extracted content, naming the limit it hit.

#### Scenario: A traversal entry is refused

- **WHEN** an archive contains `../outside/SKILL.md`
- **THEN** extraction is refused and no file is written outside the extraction root

#### Scenario: Links are never created

- **WHEN** an archive contains a symbolic or hard link entry
- **THEN** no link is created and the skill it belongs to is not installed through it

#### Scenario: An oversized archive is refused

- **WHEN** an archive declares more than 1000 entries or expands beyond 25 MiB
- **THEN** extraction stops with an error naming the limit and nothing is installed

#### Scenario: Non-skill content is not written

- **WHEN** a repository archive contains sources, documentation, and a `skills/` directory
- **THEN** only the skill directories are written; the rest is not extracted into the project

### Requirement: An install reports what it wrote and never overwrites silently

An install SHALL report each skill as installed, already present unchanged, or present with different content, and SHALL NOT replace a present skill unless the request explicitly asks for replacement. A skill the flow requires but the source does not provide SHALL be reported missing rather than substituted under another name.

#### Scenario: Reinstalling the same content

- **WHEN** an install runs twice from the same source
- **THEN** the second run reports the skills as unchanged and their bytes are identical

#### Scenario: Different content is reported, not replaced

- **WHEN** a skill directory exists with different content and replacement was not requested
- **THEN** the skill is reported as present with different content and left untouched

#### Scenario: Replacement is explicit

- **WHEN** the request asks to replace
- **THEN** the directory's content is replaced and the report says so

#### Scenario: A partial source leaves the rest missing

- **WHEN** the source provides one of the four skills a flow binds
- **THEN** the report lists the one installed and the three still missing

### Requirement: The online install runs the skills CLI

When offline mode is not active and no local source is configured, an install SHALL run the `skills` CLI against a pinned release of the OpenSpec skills, targeting the shared `.agents/skills` directory, with telemetry disabled, and SHALL report the command's output and exit status.

#### Scenario: The online install names its source

- **WHEN** an install runs with no local source and offline mode off
- **THEN** the command names the pinned OpenSpec release and the shared `.agents/skills` target

#### Scenario: A failing command is reported

- **WHEN** the CLI exits non-zero
- **THEN** the response carries the exit status and captured output and readiness is unchanged

#### Scenario: Offline mode never builds the command

- **WHEN** offline mode is active and no local source is configured
- **THEN** no command is constructed, the request is refused naming offline mode, and nothing is written

### Requirement: Readiness names the actions offline mode withholds

When offline mode is active the report SHALL mark each provisioning action as available or unavailable with its reason, and SHALL offer a local skills source as the available path so an operator can see what to provide.

#### Scenario: Offline with no source

- **WHEN** offline mode is active and no skills source is configured
- **THEN** the report marks the skills install unavailable with offline mode as the reason

#### Scenario: Offline with a source

- **WHEN** offline mode is active and a local source is configured
- **THEN** the report marks the local install available and the online install unavailable

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
