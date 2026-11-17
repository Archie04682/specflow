# Proposal

## Why

The interaction dock renders a pending question's options as single-line buttons, so a long option label
overflows around its centered span and is clipped on both sides by the card — the operator reads a middle
fragment of the text they are choosing. The dock is also unbounded in height: a question with several
long options takes most of the activity column and squeezes the Activity window the operator needs in
order to answer it. Both defects are visible in the repo's own run journals, where select questions
arrive with five or six options of 60-200 characters each.

## What Changes

- **Long option labels wrap instead of clipping.** A select option is a left-aligned, multi-line label
  that breaks at word boundaries and inside long unbroken tokens, so the whole answer text stays
  readable at any activity-column width.
- **The dock yields at least half its column to the Activity window.** The interaction dock takes at most
  half of the activity column's height (less the pane gap); the Activity window keeps the rest instead of
  collapsing toward zero. The bound applies to whatever the dock shows — question, approval, or
  clarification — in both the side-by-side and stacked layouts.
- **The question card scrolls as one region.** When the pending interaction does not fit the bound, the
  card scrolls its own content, keeping title, description, options, and the cancel action reachable with
  a single scrollbar rather than nested ones.

## Capabilities

### New Capabilities

<!-- None: the change refines an existing capability's interaction-dock presentation. -->

- None.

### Modified Capabilities

- `run-dashboard`: adds a presentation requirement that bounds the interaction dock's height to half the
  activity column, scrolls a question that does not fit, and wraps long option labels so no option text is
  clipped or elided.

## Impact

- `frontend/src/components/RunDashboard/InteractionPanel/AskCard.tsx` — option buttons carry an option
  label class; no change to the answer payloads, the question kinds, or the submit path.
- `frontend/src/styles/globals.css` — the run-side dock bound, the scrollable interaction card, and the
  wrapping option label.
- `frontend/src/components/RunDashboard/InteractionPanel/AskCard.test.tsx` — a regression case for a long
  option label.
- No backend, API, adapter, or dependency change; no new state, no stored preference, and no JavaScript
  layout calculation.
