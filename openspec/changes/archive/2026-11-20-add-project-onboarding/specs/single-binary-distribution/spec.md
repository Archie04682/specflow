# Spec Delta

## ADDED Requirements

### Requirement: The executable carries its workflow templates

The executable SHALL carry the workflow templates it seeds, compiled into the
artifact, and a seed SHALL read them from the executable rather than from a
source checkout or the network. Running the product SHALL NOT require the
SpecFlow repository to be present on the machine.

#### Scenario: Seeding from a moved executable

- **WHEN** the executable runs outside any SpecFlow checkout and a project seeds
  its default flow
- **THEN** the template comes from the executable and the written file is
  identical to the one the same build embeds

#### Scenario: The embedded set is named

- **WHEN** a client asks which templates the build carries
- **THEN** the default flow's reference is listed without reading the disk

### Requirement: Offline mode disables network provisioning

The executable SHALL accept an offline mode through a `--offline` flag or a
`SPECFLOW_OFFLINE=1` environment variable, activated by either. While offline
mode is active the process SHALL construct no command that reaches the network —
no package manager, no `npx`, no remote git — and SHALL report a dependency it
cannot install locally instead of fetching it.

#### Scenario: Offline refuses a network install

- **WHEN** offline mode is active and a client asks to install skills with no
  local source
- **THEN** no command is constructed, the request is refused naming offline
  mode, and nothing is written

#### Scenario: The environment activates it

- **WHEN** the process starts with `SPECFLOW_OFFLINE=1` and no flag
- **THEN** network provisioning is refused exactly as with the flag

#### Scenario: Offline keeps local work working

- **WHEN** offline mode is active and a project seeds a flow and starts a run
- **THEN** seeding, the worktree, the run, and its gates behave as they do online
