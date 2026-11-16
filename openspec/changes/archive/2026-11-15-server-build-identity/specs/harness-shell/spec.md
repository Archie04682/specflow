# Spec Delta

## ADDED Requirements

### Requirement: The shell exposes the running server's build

The shell SHALL read the server identity on mount and again when the window
regains focus. Settings SHALL show the running build's revision and start time,
plus the checkout state when the server reports one. A failed identity read
SHALL leave the workbench usable and SHALL NOT surface a blocking error.

#### Scenario: Settings names the running build

- **WHEN** the identity reports a revision and a process start time
- **THEN** Settings shows both for the running server

#### Scenario: An unavailable identity is not an error

- **WHEN** the identity request fails
- **THEN** the shell still renders and no identity error blocks the workbench

#### Scenario: Focus refreshes the identity

- **WHEN** the window regains focus and the server now reports a different start
  time
- **THEN** Settings shows the new build without a page reload

### Requirement: A stale server build is visible

When the identity reports that the running build is behind the checkout or
older than its backend sources, the workbench SHALL show a warning banner
naming the running build and telling the operator to restart the server, and
SHALL let the operator dismiss it. The banner SHALL NOT appear for a current
build, an unavailable identity, or a dismissed warning for the same server
process.

#### Scenario: A stale build warns

- **WHEN** the identity reports a checkout ahead of the running build
- **THEN** a warning banner states that the running build is older than the
  checkout and that the server must restart

#### Scenario: A current build is silent

- **WHEN** the identity reports no staleness
- **THEN** no stale-build banner is shown

#### Scenario: Dismissal is per server process

- **WHEN** the operator dismisses the warning
- **THEN** it stays hidden while that server process is running, and a warning
  reported by a different server process is shown again

#### Scenario: An unavailable identity is silent

- **WHEN** the identity cannot be read
- **THEN** no stale-build banner is shown
