import { useRef, useState } from "react";
import {
  normalizeModel,
  parseTriState,
  routeEntries,
  triStateValue,
  withRoute,
  withoutRoute,
} from "../../editor/model";
import { PROMPT_VARIABLES } from "../../editor/promptVariables";
import type { HarnessInfo } from "../../types/harness";
import type { NodeKind } from "../../types/run";
import type { SkillInfo } from "../../types/skill";
import type { WorkflowDraft, WorkflowNodeDraft } from "../../types/workflowDraft";
import { Button } from "../common/Button";
import { HarnessModelFields } from "./HarnessModelFields";
import { MultiSelectField, type MultiSelectOption } from "./MultiSelectField";

interface NodeInspectorProps {
  draft: WorkflowDraft;
  node: WorkflowNodeDraft;
  skills?: SkillInfo[] | null;
  skillsError?: string | null;
  harnesses?: HarnessInfo[] | null;
  harnessesError?: string | null;
  onChange: (patch: Partial<WorkflowNodeDraft>) => void;
  onRemove: () => void;
  onClose: () => void;
}

// nextRouteCode picks a free exit code so adding a route never overwrites one.
function nextRouteCode(node: WorkflowNodeDraft): number {
  const entries = routeEntries(node);
  if (entries.length === 0) return 0;
  return Math.max(...entries.map((entry) => entry.code)) + 1;
}

const POSTCONDITION_TYPES = [
  "none",
  "file_exists",
  "new_change",
  "tasks_complete",
  "exit_zero",
] as const;

export function NodeInspector({
  draft,
  node,
  skills,
  skillsError,
  harnesses,
  harnessesError,
  onChange,
  onRemove,
  onClose,
}: NodeInspectorProps) {
  const [toolsText, setToolsText] = useState(node.tools?.join(", ") ?? "");
  const [skillsText, setSkillsText] = useState(
    (node.skills?.length ? node.skills : node.skill ? [node.skill] : []).join(", "),
  );
  const promptRef = useRef<HTMLTextAreaElement>(null);
  const model = node.model ?? {};
  const catalogTools = harnesses?.find(
    (harness) =>
      harness.id === (node.harness?.trim() || draft.defaults.harness?.trim()),
  )?.tools;
  const nodeHarnessId = node.harness?.trim() || draft.defaults.harness?.trim();
  const postcondition = node.postcondition;
  const skillCatalog = skills ?? null;
  const selectedSkills = node.skills?.length
    ? node.skills
    : node.skill
      ? [node.skill]
      : [];
  const installedSkillNames = new Set(skillCatalog?.map((skill) => skill.name));
  const skillOptions: MultiSelectOption[] = [
    ...(skillCatalog ?? []).map((skill) => ({
      value: skill.name,
      label: skill.name,
      group: skill.source,
    })),
    ...selectedSkills
      .filter((name) => !installedSkillNames.has(name))
      .map((name) => ({
        value: name,
        label: name,
        group: "Not installed",
      })),
  ];
  const missingSkills = selectedSkills.filter(
    (name) => !installedSkillNames.has(name),
  );
  const harnessPlaceholder = draft.defaults.harness
    ? `default: ${draft.defaults.harness}`
    : "required for an agent node";

  return (
    <section
      className="panel editor-section"
      aria-label={`${node.title ?? node.id} settings`}
    >
      <header className="panel__header">
        <h2 className="panel__title">Node</h2>
        <Button variant="ghost" size="sm" onClick={onClose}>
          Close
        </Button>
      </header>
      <div className="panel__body editor-form">
        <div className="field">
          <label className="field__label" htmlFor="node-kind">
            Kind
          </label>
          <select
            id="node-kind"
            className="input"
            value={node.kind}
            onChange={(event) =>
              onChange({ kind: event.target.value as NodeKind })
            }
          >
            <option value="agent">agent</option>
            <option value="command">command</option>
          </select>
        </div>
        <div className="field">
          <label className="field__label" htmlFor="node-title">
            Title
          </label>
          <input
            id="node-title"
            className="input"
            value={node.title ?? ""}
            onChange={(event) => onChange({ title: event.target.value })}
          />
        </div>

        {node.kind === "agent" ? (
          <>
            <div className="field">
              <label className="field__label" htmlFor="node-harness">
                Harness
              </label>
              <input
                id="node-harness"
                className="input mono"
                placeholder={harnessPlaceholder}
                value={node.harness ?? ""}
                onChange={(event) =>
                  onChange({ harness: event.target.value || undefined })
                }
              />
            </div>
            <div className="field">
              {skillCatalog ? (
                <MultiSelectField
                  id="node-skill"
                  label="Skills"
                  options={skillOptions}
                  value={selectedSkills}
                  onChange={(values) =>
                    onChange({
                      skills: values.length > 0 ? values : undefined,
                      skill: undefined,
                    })
                  }
                  placeholder="No skills"
                  emptyText="No skills installed in the configured roots."
                  hint={
                    missingSkills.length > 0
                      ? `Not installed: ${missingSkills.join(", ")}. The run will reject an uninstalled skill.`
                      : undefined
                  }
                />
              ) : (
                <>
                  <label className="field__label" htmlFor="node-skill">
                    Skills
                  </label>
                  <input
                    id="node-skill"
                    className="input mono"
                    placeholder="skill-a, skill-b"
                    value={skillsText}
                    aria-describedby={
                      skillsError ? "node-skill-hint" : undefined
                    }
                    onChange={(event) => setSkillsText(event.target.value)}
                    onBlur={() => {
                      const names = skillsText
                        .split(",")
                        .map((entry) => entry.trim())
                        .filter(Boolean);
                      onChange({
                        skills: names.length > 0 ? names : undefined,
                        skill: undefined,
                      });
                    }}
                  />
                  {skillsError ? (
                    <p className="field__hint" id="node-skill-hint">
                      {`Could not list installed skills: ${skillsError}. Enter names manually.`}
                    </p>
                  ) : null}
                </>
              )}
            </div>

            <div className="field">
              <label className="field__label" htmlFor="node-prompt">
                Prompt
              </label>
              <textarea
                id="node-prompt"
                ref={promptRef}
                className="input"
                rows={5}
                value={node.prompt ?? ""}
                onChange={(event) => onChange({ prompt: event.target.value })}
                aria-describedby="node-prompt-hint"
              />
              <div className="prompt-variables" aria-label="Prompt variables">
                <span className="prompt-variables__label">Variables</span>
                {PROMPT_VARIABLES.map((variable) => (
                  <button
                    key={variable.token}
                    type="button"
                    className="prompt-variable"
                    title={variable.description}
                    aria-label={`Insert {{${variable.token}}}`}
                    onClick={() => {
                      const textarea = promptRef.current;
                      const text = node.prompt ?? "";
                      const start = textarea?.selectionStart ?? text.length;
                      const end = textarea?.selectionEnd ?? start;
                      const token = `{{${variable.token}}}`;
                      onChange({
                        prompt: `${text.slice(0, start)}${token}${text.slice(end)}`,
                      });
                      requestAnimationFrame(() => {
                        textarea?.focus();
                        textarea?.setSelectionRange(
                          start + token.length,
                          start + token.length,
                        );
                      });
                    }}
                  >
                    {`{{${variable.token}}}`}
                  </button>
                ))}
              </div>
              <p className="field__hint" id="node-prompt-hint">
                Click a variable to insert it at the cursor. Unknown
                placeholders fail compilation before the node starts.
              </p>
            </div>

            <HarnessModelFields
              idPrefix="node"
              model={model}
              harnesses={harnesses ?? null}
              harnessId={nodeHarnessId}
              harnessesError={harnessesError}
              scope="node"
              onChange={(next) => onChange({ model: normalizeModel(next) })}
            />

            {catalogTools && catalogTools.length > 0 ? (
              <MultiSelectField
                id="node-tools"
                label="Tools"
                options={catalogTools.map((tool) => ({
                  value: tool.name,
                  label: tool.name,
                  description: tool.description,
                }))}
                value={node.tools ?? []}
                onChange={(tools) =>
                  onChange({ tools: tools.length > 0 ? tools : undefined })
                }
                placeholder="No tools (harness default)"
                hint="No selection means the harness default tools. Add a custom name if the catalog misses a tool."
                custom={{
                  placeholder: "Custom tool name",
                  addLabel: "Add a custom tool",
                }}
              />
            ) : (
              <div className="field">
                <label className="field__label" htmlFor="node-tools">
                  Tools
                </label>
                <input
                  id="node-tools"
                  className="input mono"
                  placeholder="read, write"
                  value={toolsText}
                  onChange={(event) => setToolsText(event.target.value)}
                  onBlur={() => {
                    const tools = toolsText
                      .split(",")
                      .map((tool) => tool.trim())
                      .filter(Boolean);
                    onChange({ tools: tools.length > 0 ? tools : undefined });
                  }}
                />
                <p className="field__hint">
                  Comma-separated; empty means the harness default.
                </p>
              </div>
            )}

            <div className="field-grid">
              <div className="field">
                <label className="field__label" htmlFor="node-interactive">
                  Interactive
                </label>
                <select
                  id="node-interactive"
                  className="input"
                  value={triStateValue(node.interactive)}
                  onChange={(event) =>
                    onChange({ interactive: parseTriState(event.target.value) })
                  }
                >
                  <option value="">Inherit</option>
                  <option value="true">On</option>
                  <option value="false">Off</option>
                </select>
              </div>
              <div className="field">
                <label className="field__label" htmlFor="node-inject">
                  Allow inject
                </label>
                <select
                  id="node-inject"
                  className="input"
                  value={triStateValue(node.allowInject)}
                  onChange={(event) =>
                    onChange({ allowInject: parseTriState(event.target.value) })
                  }
                >
                  <option value="">Inherit</option>
                  <option value="true">On</option>
                  <option value="false">Off</option>
                </select>
              </div>
            </div>
          </>
        ) : (
          <div className="field">
            <label className="field__label" htmlFor="node-command">
              Command
            </label>
            <textarea
              id="node-command"
              className="input mono"
              rows={5}
              value={node.command ?? ""}
              onChange={(event) => onChange({ command: event.target.value })}
            />
          </div>
        )}

        {node.kind === "command" ? (
          <fieldset className="editor-fieldset">
            <legend>Routes</legend>
            <p className="muted small">
              A declared exit code completes this node under its label instead
              of failing it, and selects the outgoing edge carrying that label.
              Any other exit code still fails the node.
            </p>
            {routeEntries(node).map(({ code, label }) => (
              <div className="route-row" key={code}>
                <div className="field">
                  <label
                    className="field__label"
                    htmlFor={`node-route-code-${code}`}
                  >
                    Exit code
                  </label>
                  <input
                    id={`node-route-code-${code}`}
                    className="input mono"
                    type="number"
                    value={code}
                    onChange={(event) => {
                      const parsed = Number(event.target.value);
                      if (!Number.isFinite(parsed)) return;
                      const moved = { ...(withoutRoute(node, code) ?? {}) };
                      moved[String(parsed)] = label;
                      onChange({ routes: moved });
                    }}
                  />
                </div>
                <div className="field">
                  <label
                    className="field__label"
                    htmlFor={`node-route-label-${code}`}
                  >
                    Route
                  </label>
                  <input
                    id={`node-route-label-${code}`}
                    className="input mono"
                    placeholder="clean"
                    value={label}
                    onChange={(event) =>
                      onChange({ routes: withRoute(node, code, event.target.value) })
                    }
                  />
                </div>
                <div className="field route-row__action">
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => onChange({ routes: withoutRoute(node, code) })}
                  >
                    Remove
                  </Button>
                </div>
              </div>
            ))}
            <div className="editor-actions">
              <Button
                variant="secondary"
                size="sm"
                onClick={() =>
                  onChange({
                    routes: withRoute(node, nextRouteCode(node), ""),
                  })
                }
              >
                Add route
              </Button>
            </div>
          </fieldset>
        ) : null}

        <div className="field-grid">
          <div className="field">
            <label className="field__label" htmlFor="node-workdir">
              Working directory
            </label>
            <select
              id="node-workdir"
              className="input"
              value={node.workdir ?? ""}
              onChange={(event) =>
                onChange({ workdir: event.target.value || undefined })
              }
            >
              <option value="">Default (run)</option>
              <option value="run">run</option>
              <option value="project">project</option>
            </select>
          </div>
          <div className="field">
            <label className="field__label" htmlFor="node-timeout">
              Timeout
            </label>
            <input
              id="node-timeout"
              className="input mono"
              placeholder="30m"
              value={node.timeout ?? ""}
              onChange={(event) =>
                onChange({ timeout: event.target.value || undefined })
              }
            />
          </div>
        </div>

        <fieldset className="editor-fieldset">
          <legend>Interaction</legend>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={node.requireApproval ?? false}
              onChange={(event) =>
                onChange({ requireApproval: event.target.checked })
              }
            />
            Require approval before every attempt
          </label>

          <div className="field-grid">
            <div className="field">
              <label className="field__label" htmlFor="node-ask-timeout">
                Ask timeout
              </label>
              <input
                id="node-ask-timeout"
                className="input mono"
                placeholder="inherit"
                value={node.askTimeout ?? ""}
                onChange={(event) =>
                  onChange({ askTimeout: event.target.value || undefined })
                }
              />
            </div>
            <div className="field">
              <label className="field__label" htmlFor="node-approval-timeout">
                Approval timeout
              </label>
              <input
                id="node-approval-timeout"
                className="input mono"
                placeholder="inherit"
                value={node.approvalTimeout ?? ""}
                onChange={(event) =>
                  onChange({ approvalTimeout: event.target.value || undefined })
                }
              />
            </div>
          </div>

          <div className="field">
            <label className="field__label" htmlFor="node-retry">
              Retry attempts
            </label>
            <input
              id="node-retry"
              className="input"
              type="number"
              min={0}
              value={node.retry ?? ""}
              onChange={(event) => {
                const parsed = Number(event.target.value);
                onChange({
                  retry: Number.isFinite(parsed) ? parsed : undefined,
                });
              }}
            />
          </div>
        </fieldset>

        <fieldset className="editor-fieldset">
          <legend>Postcondition</legend>
          {postcondition ? (
            <>
              <div className="field">
                <label className="field__label" htmlFor="node-post-type">
                  Type
                </label>
                <select
                  id="node-post-type"
                  className="input"
                  value={postcondition.type}
                  onChange={(event) =>
                    onChange({
                      postcondition: {
                        ...postcondition,
                        type: event.target.value,
                      },
                    })
                  }
                >
                  {POSTCONDITION_TYPES.map((type) => (
                    <option key={type} value={type}>
                      {type}
                    </option>
                  ))}
                </select>
              </div>

              {postcondition.type === "file_exists" ? (
                <div className="field">
                  <label className="field__label" htmlFor="node-post-path">
                    Path
                  </label>
                  <input
                    id="node-post-path"
                    className="input mono"
                    value={postcondition.path ?? ""}
                    onChange={(event) =>
                      onChange({
                        postcondition: {
                          ...postcondition,
                          path: event.target.value,
                        },
                      })
                    }
                  />
                </div>
              ) : null}

              {postcondition.type === "new_change" ||
              postcondition.type === "tasks_complete" ? (
                <div className="field">
                  <label className="field__label" htmlFor="node-post-change">
                    Change
                  </label>
                  <input
                    id="node-post-change"
                    className="input mono"
                    placeholder="{{change}}"
                    value={postcondition.change ?? ""}
                    onChange={(event) =>
                      onChange({
                        postcondition: {
                          ...postcondition,
                          change: event.target.value,
                        },
                      })
                    }
                  />
                </div>
              ) : null}

              <div className="field">
                <label className="field__label" htmlFor="node-post-description">
                  Description
                </label>
                <input
                  id="node-post-description"
                  className="input"
                  value={postcondition.description ?? ""}
                  onChange={(event) =>
                    onChange({
                      postcondition: {
                        ...postcondition,
                        description: event.target.value,
                      },
                    })
                  }
                />
              </div>

              <Button
                variant="ghost"
                size="sm"
                onClick={() => onChange({ postcondition: undefined })}
              >
                Remove postcondition
              </Button>
            </>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onChange({ postcondition: { type: "none" } })}
            >
              Add postcondition
            </Button>
          )}
        </fieldset>

        <div className="editor-actions">
          <Button variant="danger" size="sm" onClick={onRemove}>
            Remove node
          </Button>
        </div>
      </div>
    </section>
  );
}
