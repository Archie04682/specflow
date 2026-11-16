# editor-value-pickers Specification

## Purpose

Lets the operator choose supported models, thinking levels, tools, skills, and prompt variables from harness and server catalogs, while keeping manual entry available whenever a catalog cannot be read.

## Requirements

### Requirement: Model is chosen from the harness catalog

The node inspector and workflow defaults SHALL render the model as a grouped
select when the resolved harness reports models. The select SHALL include an
inherit option, the explicit harness-default sentinel, and a preserved current
value that is not in the catalog. Without a catalog the field SHALL remain an
editable text input with the reason shown.

#### Scenario: Selecting a model
- **WHEN** the operator chooses a catalog model
- **THEN** the model value is stored and the thinking options match that model

#### Scenario: No catalog
- **WHEN** the harness reports no catalog
- **THEN** the model stays a manual input with a hint

### Requirement: Thinking is constrained by the selected model

Thinking SHALL be a select populated from the selected model's levels, falling
back to the harness-wide levels. Changing the model SHALL drop a thinking level
the new model does not support. Thinking SHALL be disabled with an explanation
when the model or harness cannot reason.

#### Scenario: Changing to a non-reasoning model
- **WHEN** the new model reports no reasoning support and the old model had a
  thinking level
- **THEN** the thinking level is cleared and the field is disabled

#### Scenario: Supported levels
- **WHEN** a reasoning model is selected
- **THEN** only its reported thinking levels are offered

### Requirement: Tools are selected from a multi-select dropdown

When the harness reports tools, the inspector SHALL render a multi-select
dropdown whose trigger shows the selected tools as chips and whose panel lists
the catalog tools as checkboxes. The panel SHALL offer a custom-name entry and
a clear action. An empty selection SHALL write no tools so the harness default
applies. Selected names outside the catalog SHALL remain visible and removable.
Without a tool catalog the comma-separated text field SHALL remain.

#### Scenario: Selecting a tool
- **WHEN** the operator checks a tool in the dropdown
- **THEN** the node's tool list contains it and the trigger shows its chip

#### Scenario: Selecting several tools
- **WHEN** the operator checks more tools without closing the panel
- **THEN** every checked tool is in the node's tool list

#### Scenario: Clearing the allowlist
- **WHEN** the operator clears all selected tools
- **THEN** the node stores no tool list

#### Scenario: A custom tool
- **WHEN** the operator adds a name that is not in the catalog
- **THEN** the node's tool list contains that name

### Requirement: Skills are selected from a multi-select dropdown

When the skill catalog is available, the inspector SHALL render a multi-select
dropdown grouped by source, with every selected skill shown as a chip. It SHALL
support selecting several skills in one node. A selected skill that is not
installed SHALL remain visible and marked. The inspector SHALL write the plural
`skills` list and SHALL NOT also write the singular `skill`. Without a skill
catalog the comma-separated skill input SHALL remain.

#### Scenario: Selecting several skills
- **WHEN** the operator checks two installed skills
- **THEN** the node stores both in order and the trigger shows both chips

#### Scenario: An uninstalled skill
- **WHEN** a node already names a skill that is not installed
- **THEN** that value stays selected and the inspector warns that the run will
  reject it

#### Scenario: Catalog unavailable
- **WHEN** the skill catalog cannot be listed
- **THEN** the input remains manual and accepts a comma-separated list

### Requirement: The prompt shows the insertable run variables

The prompt field SHALL show the closed set of render variables as clickable
chips: `objective`, `change`, `run_id`, and `workdir`. Activating a chip SHALL
insert `{{token}}` at the caret and keep editing focus. The hint SHALL state
that unknown placeholders fail compilation. The editor SHALL NOT introduce a
template language beyond the server's closed set.

#### Scenario: Inserting a variable
- **WHEN** the operator activates the objective chip
- **THEN** `{{objective}}` is inserted at the caret

#### Scenario: The full set is visible
- **WHEN** the prompt section renders
- **THEN** all four supported variables are shown
