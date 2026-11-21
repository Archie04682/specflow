# single-binary-distribution Specification

## Purpose

Deliver SpecFlow as a single self-contained executable so users can run the product without installing a separate web server, a JavaScript runtime, or unpacking static assets.

## Requirements

### Requirement: Single executable artifact

The build SHALL produce one executable artifact that contains both the HTTP API and the compiled web UI. Running the product SHALL NOT require Node.js, a separate web server, or any runtime asset outside the executable.

#### Scenario: Build produces one artifact
- **WHEN** the release build completes
- **THEN** a single `specflow` executable exists and is the only artifact needed to run the product

#### Scenario: Runs without a JavaScript runtime
- **WHEN** the executable is started on a machine that has no Node.js installed
- **THEN** the web UI loads normally

### Requirement: Web UI served from the executable

The executable SHALL serve the compiled web UI at the root path and SHALL return the UI entry document for unknown non-API GET paths so client-side routes resolve on a fresh page load.

#### Scenario: Root path serves the UI
- **WHEN** the client sends `GET /`
- **THEN** the response is `200` with an HTML document that mounts the application

#### Scenario: Unknown client route falls back to the entry document
- **WHEN** the client sends `GET /flows/example` and no such static file exists
- **THEN** the response is `200` with the UI entry document

#### Scenario: Unknown API path returns an error, not the UI
- **WHEN** the client sends `GET /api/does-not-exist`
- **THEN** the response is `404` with a JSON error body

### Requirement: Health endpoint

The executable SHALL expose `GET /api/health` returning `200` with a JSON body that reports the service status.

#### Scenario: Health check succeeds
- **WHEN** the client sends `GET /api/health`
- **THEN** the response is `200` with `Content-Type: application/json` and a body containing `"status":"ok"`

### Requirement: Configurable listen port

The executable SHALL listen on TCP port `8080` by default, SHALL accept an override through a `--port` flag or a `PORT` environment variable, and SHALL give the flag precedence when both are provided.

#### Scenario: Default port
- **WHEN** the executable starts with no port configuration
- **THEN** it listens on port `8080`

#### Scenario: Port overridden by flag
- **WHEN** the executable starts with `--port 9090`
- **THEN** it listens on port `9090`

#### Scenario: Port overridden by environment
- **WHEN** the executable starts with `PORT=9091` and no flag
- **THEN** it listens on port `9091`

#### Scenario: Flag takes precedence over environment
- **WHEN** the executable starts with `PORT=9091` and `--port 9090`
- **THEN** it listens on port `9090`

#### Scenario: Invalid port is rejected
- **WHEN** the executable starts with `--port not-a-number`
- **THEN** it exits with a non-zero status and a message that names the invalid value

### Requirement: Server build identity

The executable SHALL expose `GET /api/server` returning `200` with a JSON body identifying the running build: service name, source revision, revision time, modified flag, and process start time. A value the build cannot determine SHALL be omitted rather than guessed. The endpoint SHALL NOT fail because a stamp is missing.

#### Scenario: Identity names the running build
- **WHEN** the client sends `GET /api/server` to a build made from a known revision
- **THEN** the response is `200` with `Content-Type: application/json` and a body carrying the service name, that revision, and the process start time

#### Scenario: Unknown stamps are omitted
- **WHEN** the executable carries no revision or build time
- **THEN** the response omits those fields and still returns `200`

#### Scenario: Identity does not replace health
- **WHEN** the client sends `GET /api/health`
- **THEN** the response is unchanged: `200` with `"status":"ok"`

### Requirement: Development builds identify themselves

An executable produced by the development workflow SHALL report its source revision and build time even though the Go toolchain records no version-control settings for such a build. When no stamp is injected, identity SHALL fall back to the toolchain's build settings.

#### Scenario: A development server reports its revision
- **WHEN** the server is started by the development workflow from a git checkout
- **THEN** `GET /api/server` reports the checkout revision captured at build time and the build time

#### Scenario: A plain build falls back to the toolchain
- **WHEN** the executable is built without the workflow stamp and the Go toolchain recorded version-control settings
- **THEN** identity reports the toolchain's revision and revision time

### Requirement: A checkout behind the running build is reported

When the server runs from a SpecFlow source checkout, the identity endpoint SHALL report whether that checkout has moved past the running build — a different HEAD, or backend sources modified after the build — and SHALL name a reason. This check SHALL be best-effort: an unrecognized tree SHALL omit the report, unreadable version-control state SHALL omit only the version-control findings, and neither SHALL fail the endpoint.

#### Scenario: The checkout HEAD moved past the build
- **WHEN** the running build was made from revision A and the checkout's HEAD is a descendant revision B
- **THEN** the response reports the checkout as behind the running build and names revision B

#### Scenario: Sources are newer than the build
- **WHEN** a backend source file was modified after the build and the checkout HEAD is unchanged
- **THEN** the response reports the running build as stale, naming its reason

#### Scenario: Unreadable version control keeps the source finding
- **WHEN** the checkout's version-control state cannot be read and a backend source is newer than the build
- **THEN** the response reports the running build as stale from its sources and omits the checkout revision

#### Scenario: Not a source checkout
- **WHEN** the server runs outside a recognized SpecFlow checkout
- **THEN** the response omits the checkout report and returns `200`

#### Scenario: A probe failure does not fail the endpoint
- **WHEN** the checkout or its version-control state cannot be read
- **THEN** the endpoint still returns `200` with the identity it has

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
