# Spec Delta

## Purpose

Deliver SpecFlow as a single self-contained executable so users can run the product without installing a separate web server, a JavaScript runtime, or unpacking static assets.

## ADDED Requirements

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
