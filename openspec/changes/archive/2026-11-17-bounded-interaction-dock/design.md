# Design

See proposal.md for motivation and the `run-dashboard` delta for the required behavior.

## Context

The run view puts the Activity window and the interaction dock in one flex column
(`.run-side`): the stream panel is `flex: 1; min-height: 0`, the dock is `flex: 0 0 auto` and has no
height bound, so the dock's content decides how much the stream loses. The dock renders exactly one card
— a question, an approval, or the clarification box — and each card class sets `overflow: hidden`.

Select options are ordinary `Button`s. `.button` is `white-space: nowrap` with a centered inline-flex
span, and the option container is a column flex box with `align-items: stretch`, so a long label is
stretched to the card's width as a box but laid out on one line, overflowing symmetrically around the
center and clipped at the card edge.

Styling lives in one plain stylesheet (`frontend/src/styles/globals.css`) under a token set; layout is
not computed in JavaScript, and the test harness is jsdom, which applies no stylesheet and computes no
layout. There is no viewport-measuring hook in this view apart from `RunSplit`, which measures the
column width for the horizontal split.

## Goals / Non-Goals

**Goals:**

- The dock's height is a function of the activity column, not of the question's length.
- The operator can read every option label in full at any column width.
- One scroll container inside the dock; the layout keeps working when the bound is smaller than the
  card's own heading and actions.

**Non-Goals:**

- A second adjustable separator inside the activity column (`RunSplit` stays the only one).
- Pinning the card's heading or its action row while the body scrolls.
- Changing option semantics (still one click, still sends the option text), the ask kinds, the approval
  card's contents, or the activity stream's own rendering.
- Eliding an option with an ellipsis or a tooltip; a truncated answer label is not acceptable.

## Decisions

**Bound the dock, not the card.** A percentage `max-height` resolves against its containing block's
height. The dock's containing block is `.run-side`, which is a stretched grid item with a definite
height; the card's would be the dock, whose height is content-derived unless the dock itself is
constrained. So the cap goes on `.interaction-dock`, which becomes a flex column with
`flex: 0 1 auto; min-height: 0; max-height: calc(50% - 6px)`, and the card becomes its shrinking child
(`min-height: 0`, `overflow-y: auto`). Alternatives rejected: viewport units (`vh`), which do not track
the column and silently break when the shell chrome changes height; and a `ResizeObserver` computing a
pixel bound in JavaScript, which adds state and a second layout path to test for no visual gain.

**The cap is half the column less half the pane gap.** `.run-side` has a 12px gap, so `calc(50% - 6px)`
is exactly the height that leaves the Activity window at least as much room as the dock and meets the
"keeps at least the other half" scenario. A plain `50%` leaves the stream 6px short of half.

**The whole card scrolls.** One scroll container inside a capped dock keeps every control reachable even
when the cap is shorter than the card's heading and action row together — the case that made a pinned
head/footer design unsafe. It also needs no markup change across the four ask kinds, and it matches what
the operator asked for: a scroll for the question's options. The alternative — heading and actions
pinned, body scrolled — reads better when the cap is comfortable but collapses the body to nothing in a
short column and would need a scroll-region wrapper in every kind. If pinning is wanted later, a
`position: sticky` action row can be added without touching the bound; that is why the boundaries are
drawn this way.

**Wrap through a dedicated option class.** The wrapping rules apply to `.interaction-card__option`
(`white-space: normal; text-align: left; justify-content: flex-start; overflow-wrap: anywhere;` plus
vertical padding and a relaxed line height), not to `.button` globally — toolbars and dialogs rely on
single-line buttons — and not to the whole `--column` actions box, because "Cancel question" shares that
column and should keep its centered ghost appearance. `min-height` stays at the button's 36px so short
options keep the established hit target. `overflow-wrap: anywhere` covers long unbroken tokens such as
paths and URLs.

**Keep the horizontal axis clipped.** The card's `overflow: hidden` becomes `overflow-y: auto` with
`overflow-x: hidden` (and `min-height: 0`), so a stray wide child cannot create a second, horizontal
scrollbar. Wrapping removes the cause rather than hiding it.

**Approvals and the clarification box inherit the bound only.** Both already fit — the approval action
block clamps at 260px — so their content and layout are untouched; they simply cannot push the stream
below half any more, which is the same requirement applied to every dock occupant.

**Tests stay structural.** jsdom applies no stylesheet, so neither the height bound nor the line count
can be asserted in the existing suite. The regression guard is the wrapping contract: a long option
renders whole inside its option element and choosing it submits the full text. The height bound is
covered by its spec scenarios and a manual check at a wide and a narrow column; adding Playwright for
one stylesheet rule is not proportionate here.

## Risks / Trade-offs

- [The height bound is CSS-only, so a future stylesheet edit can regress it without failing a test] →
  The spec states it as a scenario and the design records the containing-block reason; a structural test
  cannot observe it, and a browser test suite is a separate decision.
- [A short activity column can be shorter than the card's heading and actions combined] → Whole-card
  scrolling keeps every control reachable; this is why the pinned layout was rejected.
- [The card and the stream are two scroll areas stacked 12px apart] → Each keeps its own scrollbar and the
  stream keeps at least half the column; nested scrolling inside the card is avoided by using one region.
- [Wrapped labels make a select question taller than its single-line form] → The bound caps the dock and
  the card scrolls, so length costs scrolling rather than the operator's view of the run.
- [Left-aligned multi-line labels differ from other secondary buttons] → The difference is intentional:
  an option is a value being read and compared, not a one-word command.

## Migration Plan

None. The change is presentation-only: no API, no stored preference, no data model, and no server or
adapter behavior. It lands in the same commit as the archived change, per the repository's commit
cadence.
