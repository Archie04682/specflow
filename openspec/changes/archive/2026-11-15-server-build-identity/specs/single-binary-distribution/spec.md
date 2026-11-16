# Spec Delta

## ADDED Requirements

### Requirement: Server build identity

The executable SHALL expose `GET /api/server` returning `200` with a JSON body
identifying the running build: service name, source revision, revision time,
modified flag, and process start time. A value the build cannot determine SHALL
be omitted rather than guessed. The endpoint SHALL NOT fail because a stamp is
missing.

#### Scenario: Identity names the running build

- **WHEN** the client sends `GET /api/server` to a build made from a known
  revision
- **THEN** the response is `200` with `Content-Type: application/json` and a
  body carrying the service name, that revision, and the process start time

#### Scenario: Unknown stamps are omitted

- **WHEN** the executable carries no revision or build time
- **THEN** the response omits those fields and still returns `200`

#### Scenario: Identity does not replace health

- **WHEN** the client sends `GET /api/health`
- **THEN** the response is unchanged: `200` with `"status":"ok"`

### Requirement: Development builds identify themselves

An executable produced by the development workflow SHALL report its source
revision and build time even though the Go toolchain records no version-control
settings for such a build. When no stamp is injected, identity SHALL fall back
to the toolchain's build settings.

#### Scenario: A development server reports its revision

- **WHEN** the server is started by the development workflow from a git
  checkout
- **THEN** `GET /api/server` reports the checkout revision captured at build
  time and the build time

#### Scenario: A plain build falls back to the toolchain

- **WHEN** the executable is built without the workflow stamp and the Go
  toolchain recorded version-control settings
- **THEN** identity reports the toolchain's revision and revision time

### Requirement: A checkout behind the running build is reported

When the server runs from a SpecFlow source checkout, the identity endpoint
SHALL report whether that checkout has moved past the running build — a
different HEAD, or backend sources modified after the build — and SHALL name a
reason. This check SHALL be best-effort: an unrecognized tree SHALL omit the
report, unreadable version-control state SHALL omit only the version-control
findings, and neither SHALL fail the endpoint.

#### Scenario: The checkout HEAD moved past the build

- **WHEN** the running build was made from revision A and the checkout's HEAD is
  a descendant revision B
- **THEN** the response reports the checkout as behind the running build and
  names revision B

#### Scenario: Sources are newer than the build

- **WHEN** a backend source file was modified after the build and the checkout
  HEAD is unchanged
- **THEN** the response reports the running build as stale, naming its reason

#### Scenario: Unreadable version control keeps the source finding

- **WHEN** the checkout's version-control state cannot be read and a backend
  source is newer than the build
- **THEN** the response reports the running build as stale from its sources and
  omits the checkout revision

#### Scenario: Not a source checkout

- **WHEN** the server runs outside a recognized SpecFlow checkout
- **THEN** the response omits the checkout report and returns `200`

#### Scenario: A probe failure does not fail the endpoint

- **WHEN** the checkout or its version-control state cannot be read
- **THEN** the endpoint still returns `200` with the identity it has
