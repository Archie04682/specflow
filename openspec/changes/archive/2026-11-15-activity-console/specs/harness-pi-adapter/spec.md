# Spec Delta

## MODIFIED Requirements

### Requirement: Questions reach the user through an embedded bridge

The adapter SHALL make a question tool available to models running on Pi by loading an extension that is embedded in the executable and materialized at runtime, so that a user can answer a question without Pi's own terminal interface being present. The adapter SHALL report the question tool call's own question text as the ask's description when Pi's dialog request cannot carry it, so that the operator receives the question's context for every ask kind.

#### Scenario: Model asks and the user answers

- **WHEN** the model calls the question tool and the user submits an answer
- **THEN** the answer is returned to the model as the tool result and Pi continues the same execution

#### Scenario: Bridge extension cannot load

- **WHEN** the embedded extension cannot be loaded by the installed Pi version
- **THEN** probing reports that asking the user is unsupported and the startup failure names the incompatibility

#### Scenario: The question text reaches the operator

- **WHEN** the model calls the question tool with a question and Pi's dialog request carries only a title
- **THEN** the reported ask carries the dialog's title, kind, and options together with the question text as its description

#### Scenario: A dialog that carries its own message is unchanged

- **WHEN** Pi's dialog request already carries a message, as a confirmation does
- **THEN** the reported ask carries that message

#### Scenario: An unmatched dialog is still answerable

- **WHEN** a dialog arrives with no corresponding question tool call
- **THEN** the reported ask carries the dialog's own fields and can still be answered
