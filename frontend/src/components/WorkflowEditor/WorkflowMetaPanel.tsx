import {
  parseTriState,
  setWorkflowModel,
  triStateValue,
  updateDefaults,
  updateWorkflowFields,
} from "../../editor/model";
import type { HarnessInfo } from "../../types/harness";
import type { WorkflowDraft } from "../../types/workflowDraft";
import { HarnessModelFields } from "./HarnessModelFields";

interface WorkflowMetaPanelProps {
  draft: WorkflowDraft;
  onChange: (draft: WorkflowDraft) => void;
  nameHint?: string;
  harnesses?: HarnessInfo[] | null;
  harnessesError?: string | null;
}

export function WorkflowMetaPanel({
  draft,
  onChange,
  nameHint,
  harnesses,
  harnessesError,
}: WorkflowMetaPanelProps) {
  const model = draft.defaults.model ?? {};

  return (
    <section className="panel editor-section" aria-label="Workflow settings">
      <header className="panel__header">
        <h2 className="panel__title">Workflow</h2>
      </header>
      <div className="panel__body editor-form">
        <div className="field">
          <label className="field__label" htmlFor="wf-name">
            Name
          </label>
          <input
            id="wf-name"
            className="input"
            value={draft.name ?? ""}
            onChange={(event) =>
              onChange(updateWorkflowFields(draft, { name: event.target.value }))
            }
            placeholder="Some new flow"
          />
          {nameHint ? <p className="field__hint">{nameHint}</p> : null}
        </div>

        <div className="field">
          <label className="field__label" htmlFor="wf-description">
            Description
          </label>
          <textarea
            id="wf-description"
            className="input"
            rows={3}
            value={draft.description ?? ""}
            onChange={(event) =>
              onChange(
                updateWorkflowFields(draft, { description: event.target.value }),
              )
            }
          />
        </div>

        <fieldset className="editor-fieldset">
          <legend>Defaults</legend>
          <div className="field">
            <label className="field__label" htmlFor="wf-harness">
              Harness
            </label>
            <input
              id="wf-harness"
              className="input mono"
              value={draft.defaults.harness ?? ""}
              onChange={(event) =>
                onChange(
                  updateDefaults(draft, {
                    harness: event.target.value || undefined,
                  }),
                )
              }
            />
          </div>

          <HarnessModelFields
            idPrefix="wf"
            model={model}
            harnesses={harnesses ?? null}
            harnessId={draft.defaults.harness}
            harnessesError={harnessesError}
            scope="defaults"
            onChange={(next) => onChange(setWorkflowModel(draft, next))}
          />

          <div className="field-grid">
            <div className="field">
              <label className="field__label" htmlFor="wf-interactive">
                Interactive
              </label>
              <select
                id="wf-interactive"
                className="input"
                value={triStateValue(draft.defaults.interactive)}
                onChange={(event) =>
                  onChange(
                    updateDefaults(draft, {
                      interactive: parseTriState(event.target.value),
                    }),
                  )
                }
              >
                <option value="">Unset (off)</option>
                <option value="true">On</option>
                <option value="false">Off</option>
              </select>
            </div>
            <div className="field">
              <label className="field__label" htmlFor="wf-inject">
                Allow inject
              </label>
              <select
                id="wf-inject"
                className="input"
                value={triStateValue(draft.defaults.allowInject)}
                onChange={(event) =>
                  onChange(
                    updateDefaults(draft, {
                      allowInject: parseTriState(event.target.value),
                    }),
                  )
                }
              >
                <option value="">Unset (off)</option>
                <option value="true">On</option>
                <option value="false">Off</option>
              </select>
            </div>
          </div>

          <div className="field-grid">
            <div className="field">
              <label className="field__label" htmlFor="wf-ask-timeout">
                Ask timeout
              </label>
              <input
                id="wf-ask-timeout"
                className="input mono"
                placeholder="30m"
                value={draft.defaults.askTimeout ?? ""}
                onChange={(event) =>
                  onChange(
                    updateDefaults(draft, {
                      askTimeout: event.target.value || undefined,
                    }),
                  )
                }
              />
            </div>
            <div className="field">
              <label className="field__label" htmlFor="wf-approval-timeout">
                Approval timeout
              </label>
              <input
                id="wf-approval-timeout"
                className="input mono"
                placeholder="30m"
                value={draft.defaults.approvalTimeout ?? ""}
                onChange={(event) =>
                  onChange(
                    updateDefaults(draft, {
                      approvalTimeout: event.target.value || undefined,
                    }),
                  )
                }
              />
            </div>
          </div>
        </fieldset>
      </div>
    </section>
  );
}
