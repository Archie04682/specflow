# API

The server is unauthenticated and intended for local use. It listens on port
`8080` by default; precedence is `--port` flag > `PORT` environment variable >
`8080`, validated to 1–65535.

## Service

| Method | Path | Purpose |
| :--- | :--- | :--- |
| `GET` | `/api/health` | Returns `{"status":"ok"}` |
| `GET` | `/api/server` | Running build identity and checkout staleness |
| `GET` | `/api/harnesses` | Installed harnesses, readiness, capabilities, optional catalog |
| `GET` | `/` | The workbench SPA; unknown client routes fall back to it |

`GET /api/server` reports the service name, version, revision, revision time,
dirty flag, build time, and process start time. When launched from a SpecFlow
checkout it also reports the checkout's revision and a `staleReason` —
`checkout-ahead` or `sources-newer` — when the running build is behind.

## Runs

| Method | Path | Purpose |
| :--- | :--- | :--- |
| `POST` | `/api/runs` | Create and start a run |
| `GET` | `/api/runs?project=` | List a project's runs |
| `GET` | `/api/runs/{id}?project=` | Run, nodes, checkpoints, resolved change |
| `GET` | `/api/runs/{id}/graph?project=` | Authored nodes and edges of the pinned workflow |
| `GET` | `/api/runs/{id}/events?project=` | SSE stream, honours `Last-Event-ID` |
| `POST` | `/api/runs/{id}/answers` | Resolve a pending question |
| `POST` | `/api/runs/{id}/injections` | Send a clarification to the running node |
| `POST` | `/api/runs/{id}/cancel` | Cancel the run |
| `POST` | `/api/runs/{id}/approve` | Resolve an approval gate |
| `POST` | `/api/runs/{id}/retry` | Retry a failed node (`forward` or `clean`) |
| `POST` | `/api/runs/{id}/resume` | Continue at the first unfinished node |
| `POST` | `/api/runs/{id}/apply?project=` | Write a completed folder run's changes into the project |
| `DELETE` | `/api/runs/{id}?project=` | Discard the run's workspace (and a folder run's changes) |

Create a run:

```bash
curl -s localhost:8080/api/runs \
  -H 'content-type: application/json' \
  -d '{"project":"/path/to/repo","workflow":"standard","objective":"add an export endpoint"}'
```

A run needs a git project with a clean working tree and a workflow file. One
SpecFlow server per checkout: runs are advanced by the process that started
them, so a second server would treat another process's live run as interrupted.

## Workflows, skills, projects

| Method | Path | Purpose |
| :--- | :--- | :--- |
| `GET` | `/api/workflows?project=` | List workflows, with structural validity |
| `GET` | `/api/workflows/{reference}?project=` | Read one workflow's source, draft, and content hash |
| `PUT` | `/api/workflows/{reference}?project=` | Validate and save a workflow draft atomically |
| `GET` | `/api/skills?project=` | List installed skills from the configured roots |
| `GET` | `/api/project/readiness?project=` | Gaps, mode, actions, and whether the project can run |
| `GET` | `/api/project/repository?project=` | Whether the directory is a repository, its mode, branch, and commit |
| `POST` | `/api/project/repository` | Initialise a git repository (git mode only) |
| `POST` | `/api/project/setup` | Record the workspace mode: `{"project": "/path", "vcs": "git" \| "folder"}` |
| `POST` | `/api/project/commit` | Commit every uncommitted path; refuses secret-looking paths |
| `POST` | `/api/project/ignore` | Append paths to `.gitignore`: `{"project": "/path", "paths": [".env"]}` |
| `POST` | `/api/project/pick` | Open the native folder dialog (loopback only) |

`GET /api/workflows` returns `{"workflows": [...]}`; each entry carries
`reference`, `path`, `name`, `description`, `version`, `valid`, `problems`,
`nodes`, and `edges`. The catalog validates structurally without resolving
skills.

`GET /api/runs/{id}/graph` returns `runId`, `workflowId`, `name`, `nodes`
(declaration order, each with `id`, `title`, `kind`, `interactive`,
`allowInject`, `requireApproval`), and `edges` (`from`, `to`) from the
definition the run was pinned to. Both reads are read-only.

`POST /api/project/setup` writes `vcs` into `.specflow/config.yaml`, initialises
a repository for `git`, and creates nothing else. The mode can change only
before the project's first run.

`POST /api/project/commit` stages everything and commits it as one commit,
deriving "Initial commit" when there is no HEAD. It refuses with `409` and a
`secrets` list when a path looks like `.env`, a key, or a certificate, so a
first commit cannot publish one; the operator can then `POST
/api/project/ignore` and commit again. A clean tree is refused with `400`.

`POST /api/runs/{id}/apply` returns `{"written": [...], "deleted": [...]}` for a
completed folder run and refuses with `409` when a project file changed since
the run started. A run payload carries `vcs` and `apply` (`pending`, `applied`,
or `discarded`).

`POST /api/project/pick` opens the local operating-system folder dialog on the
machine running the server. It accepts loopback clients only and returns `501`
when the server has no desktop session or no dialog implementation (SSH, a
container, or a headless service). It does not list directories; manual path
entry stays available.

## Interactive requests

```json
POST /api/runs/{id}/answers
{"questionId": "q1", "value": "the second option", "confirmed": false}

POST /api/runs/{id}/injections
{"text": "use encoding/csv"}          -> 200 {"id": "apply-1", "status": "queued"}

POST /api/runs/{id}/approve
{"nodeId": "merge", "decision": "approve", "attempt": 1}
```

The three endpoints return `400` for a malformed or incomplete body, `404` for
an unknown run, `409` when the run or node cannot accept the operation, and
`410` when the interaction already resolved or expired. `approved: true` is still
accepted when `decision` is absent.

## Event stream

`GET /api/runs/{id}/events` is an SSE stream. It replays events after
`Last-Event-ID` (or `?after=`) and then follows the run live. Every event has
`seq`, `at`, `type`, `runId`, an optional `nodeId`, and a `data` object.

| Event | Data |
| :--- | :--- |
| `run.started` | `workflowId`, `objective` |
| `node.started` | `kind`, `title`, `attempt` (emitted per attempt) |
| `node.event` | the harness event kind and its payload |
| `node.completed` | `postcondition` (`met` or `none`), `change`, `checkpoint` |
| `node.failed`, `node.postcondition_failed` | `error`, `class`, `retryable` |
| `run.completed` | `summary` |
| `run.failed` | `error` |
| `run.cancelled` | none |
| `question.asked` | `id`, `kind`, `title`, `message`, `options`, `attempt`, `deadline` |
| `question.answered` | `id`, `resolution`, `value`, `confirmed` |
| `question.timed_out`, `question.cancelled` | `id`, `resolution` |
| `injection.received` | `id`, `text` |
| `injection.acknowledged` | `id`, `disposition`, `error` |
| `injection.rejected` | `reason`, `error` |
| `approval.required` | `id`, `node`, `title`, `kind`, `action`, `attempt`, `deadline` |
| `approval.granted`, `approval.rejected`, `approval.timed_out` | `id`, `node`, `attempt` |
| `change.resolved`, `node.skipped` | existing payloads |

A live stream ends when the run reaches `completed`, `failed`, or `cancelled`. A
client already at the terminal tail gets `204 No Content` and stops reconnecting;
a client behind the tail gets the later events and EOF; a new client gets the
full retained history. A terminal run's journal is reopened on demand. Idle live
streams receive `: keep-alive` comments.

The in-memory replay cache is bounded at 4096 events; older replay reads
`events.jsonl`. At open, a partial trailing record is discarded and a malformed
or oversized record is reported instead of silently shortening history. An
append failure is logged. The journal is not truncated; removing a run's state
with `DELETE /api/runs/{id}` removes it.

## Errors

Unknown `/api/*` paths return a JSON `404`. Harness failures carry one class from
a fixed set: `adapter_missing`, `unsupported`, `config`, `auth`, `provider`,
`start`, `protocol`, `harness_exit`, `timeout`, `cancelled`. Authentication and
configuration failures are not retryable; provider, start, protocol,
harness-exit, and timeout failures are.
