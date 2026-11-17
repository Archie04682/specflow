# Tasks

## 1. Wrap long question options

- [x] 1.1 Add the option-label rule set to `frontend/src/styles/globals.css` (wrap, left alignment, relaxed line height, vertical padding, and token breaking) and verify a 150-character option, such as the "Plugin trust model" text in `.specflow/runs/*/events.jsonl`, shows its whole label in a 360px-wide activity column with no clipping and no horizontal scrollbar.
- [x] 1.2 Apply the option-label class to each select option in `frontend/src/components/RunDashboard/InteractionPanel/AskCard.tsx` and verify the existing "sends the chosen option as the value" test still passes with `cd frontend && pnpm test -- AskCard`.
- [x] 1.3 Add a regression case to `AskCard.test.tsx` for a long option label, asserting the option element carries the whole text and that choosing it submits that full text, and verify the case fails against a deliberately truncated label and passes with `cd frontend && pnpm test -- AskCard`.

## 2. Bound the interaction dock to half the activity column

- [x] 2.1 Make the interaction dock a capped flex column in `frontend/src/styles/globals.css` (`flex: 0 1 auto`, `min-height: 0`, `max-height: calc(50% - 6px)`, column direction) and verify with a pending question taller than the bound that the activity window keeps at least half the column while a short approval keeps its natural height.
- [x] 2.2 Make the interaction card the single scroll region (`min-height: 0`, `overflow-y: auto`, `overflow-x: hidden`) and verify in a 600px-tall window that the card scrolls, its heading and options scroll together, the "Cancel question" action stays reachable, and no horizontal scrollbar appears.
- [x] 2.3 Verify the bound in the stacked layout at a viewport narrower than 1180px: the dock is still capped at half of the activity column below the graph, and the graph, activity window, and question card all remain usable.

## 3. Integration checks

- [x] 3.1 Run `make test` from the repository root and verify the Go tests, the frontend typecheck, and the Vitest suite all pass.
- [x] 3.2 Run `openspec validate bounded-interaction-dock --strict` and verify the change validates.
- [x] 3.3 Answer a waiting select question whose option wrapped in a live run view, then verify the run continues and the recorded `question.answered` event carries the option's full text as the value.

## Workflow follow-up

- Archive the change after the gates above pass, landing the implementation, the synced `run-dashboard` spec, and the archive in one commit.
