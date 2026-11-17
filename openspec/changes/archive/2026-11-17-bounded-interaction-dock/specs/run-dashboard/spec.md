# Spec Delta

## ADDED Requirements

### Requirement: The interaction dock leaves the Activity window half the column

The interaction dock SHALL take at most half of the activity column's height, counting the gap between
the panes against the dock, so the Activity window keeps at least the other half. A dock whose content
fits SHALL take only the height it needs, and content that does not fit SHALL scroll within the dock
rather than push the Activity window smaller.

#### Scenario: A tall question stops at the bound

- **WHEN** a pending question is taller than half the activity column
- **THEN** the dock stops at that bound and the question card scrolls to reveal the rest of its content

#### Scenario: The Activity window keeps the other half

- **WHEN** the dock reaches its bound
- **THEN** the Activity window keeps at least the other half of the column instead of collapsing

#### Scenario: A small interaction is not padded

- **WHEN** a pending interaction fits within the bound
- **THEN** the dock takes only the height its card needs and the Activity window takes the rest

#### Scenario: The bound holds when the panes stack

- **WHEN** the viewport is too narrow for the side-by-side layout and the activity column stacks below the graph
- **THEN** the dock is still bounded by half of that activity column

#### Scenario: A short column keeps every control reachable

- **WHEN** the activity column is too short for the card's heading and actions together
- **THEN** the card scrolls as one region and no control or option is clipped out of reach

### Requirement: A question option shows its whole label

A select question SHALL present every option's complete text: the label SHALL wrap onto as many lines as
it needs, SHALL break a long unbroken token instead of overflowing the card, and SHALL NOT be clipped,
truncated, or elided. Choosing a wrapped option SHALL send that option's full text as the answer.

#### Scenario: A long option wraps

- **WHEN** a select question offers an option longer than the dock's width
- **THEN** the option's label wraps onto further lines and its whole text is visible

#### Scenario: An unbroken token breaks

- **WHEN** an option's label contains a long unbroken token such as a path or a URL
- **THEN** the token breaks inside the label instead of running past the card's edge

#### Scenario: A wrapped option sends its whole value

- **WHEN** the operator chooses an option whose label wrapped
- **THEN** that option's full text is sent as the answer
