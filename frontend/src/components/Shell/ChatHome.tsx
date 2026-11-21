import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useHarnesses } from "../../hooks/useHarnesses";
import { useProjectReadiness } from "../../hooks/useProjectReadiness";
import { useStartRun } from "../../hooks/useRunActions";
import { usePendingAction, useRunError } from "../../stores/runStore";
import { basename } from "../../lib/format";
import type { WorkflowSummary } from "../../types/graph";
import { Banner } from "../common/Banner";
import { Button } from "../common/Button";
import {
  BrandMark,
  IconArrowUp,
  IconFlows,
  IconFolder,
  IconPlus,
} from "../common/icons";
import { ProjectOnboarding } from "./ProjectOnboarding";
import { ProjectRequired } from "./ProjectRequired";
import { ViewHeader } from "./ViewHeader";

interface ChatHomeProps {
  project: string;
  workflows: WorkflowSummary[];
  workflowsLoading: boolean;
  workflowsError: string | null;
  onReloadWorkflows: () => void;
  onStarted: (runId: string) => void;
  onNewFlow: (project: string) => void;
  onOpenProject: () => void;
  onProjectChange: (project: string) => void;
}

// The chat home is the start-run surface: the hero names the product's one
// job, the composer holds the three inputs a run needs, and the same catalog,
// readiness, and error contracts as before still decide whether it can start.
export function ChatHome({
  project,
  workflows,
  workflowsLoading,
  workflowsError,
  onReloadWorkflows,
  onStarted,
  onNewFlow,
  onOpenProject,
  onProjectChange,
}: ChatHomeProps) {
  const { harnesses, error: harnessError } = useHarnesses();
  const { readiness, loading: readinessLoading } = useProjectReadiness(project);
  const [workflow, setWorkflow] = useState("");
  const [objective, setObjective] = useState("");
  const startError = useRunError();
  const pendingAction = usePendingAction();
  const start = useStartRun();

  useEffect(() => {
    if (workflows.length === 0) return;
    const current = workflows.find((entry) => entry.reference === workflow);
    if (current?.valid) return;
    const firstValid = workflows.find((entry) => entry.valid);
    setWorkflow(firstValid?.reference ?? "");
  }, [workflows, workflow]);

  const selected = workflows.find((entry) => entry.reference === workflow);
  const invalid = useMemo(
    () => workflows.filter((entry) => !entry.valid),
    [workflows],
  );
  const hasUsableFlow = workflows.some((entry) => entry.valid);
  const notReady = harnesses.filter((harness) => !harness.ready);
  // A project that is not a repository, or has no commit, cannot start any run:
  // the setup card is the only surface worth showing there, and the catalog and
  // harness messages would only repeat it.
  const structuralGap =
    readiness !== null &&
    readiness.vcs !== "folder" &&
    (!readiness.git.repository || !readiness.git.hasCommit);
  const setupListsHarness =
    readiness !== null &&
    readiness.tools.harnesses.some((harness) => !harness.ready);
  const hasProject = project.trim() !== "";
  const canStart =
    hasProject &&
    workflow !== "" &&
    objective.trim() !== "" &&
    pendingAction === null;

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (!canStart) return;
    const result = await start(project.trim(), workflow, objective.trim());
    if (result.run) onStarted(result.run.id);
  };

  return (
    <div className="workspace-view chat-home">
      <ViewHeader
        title="New chat"
        subtitle={
          hasProject ? (
            <span className="view-header__path mono" title={project}>
              {project}
            </span>
          ) : (
            "No project selected"
          )
        }
      />

      <div className="chat-home__body">
        {!hasProject ? (
          <ProjectRequired
            title="Choose a project to begin"
            description="SpecFlow reads flows, skills, and run history from a repository's .specflow directory."
            onApply={onProjectChange}
          />
        ) : (
          <div className="chat-home__inner">
            <div className="hero">
              <div className="hero__mark" aria-hidden="true">
                <BrandMark />
              </div>
              <h2 className="hero__title">What should we build?</h2>
              <p className="hero__copy">
                Describe the outcome. SpecFlow runs a flow in its own worktree
                and pauses when it needs you.
              </p>
            </div>

            <ProjectOnboarding
              project={project}
              readiness={readiness}
              loading={readinessLoading}
              onProvisioned={onReloadWorkflows}
            />

            {startError ? <Banner tone="error">{startError}</Banner> : null}

            {workflowsError && !structuralGap ? (
              <Banner
                tone="error"
                action={
                  <Button size="sm" onClick={onReloadWorkflows}>
                    Retry
                  </Button>
                }
              >
                Could not load this project's flows: {workflowsError}
              </Banner>
            ) : null}

            {notReady.length > 0 && !structuralGap && !setupListsHarness ? (
              <Banner tone="warning">
                <strong>Harness not ready.</strong>{" "}
                {notReady
                  .map(
                    (harness) =>
                      `${harness.name}: ${harness.readyError ?? "unavailable"}`,
                  )
                  .join(" · ")}{" "}
                You can still start; the run will fail at the first node that
                needs it.
              </Banner>
            ) : null}

            {harnessError ? (
              <p className="muted small">
                Harness status unavailable: {harnessError}
              </p>
            ) : null}

            {structuralGap ? (
              <p className="chat-home__setup-hint">
                Complete the setup above, then start a run here.
              </p>
            ) : (
            <form className="composer" onSubmit={onSubmit} noValidate>
              <div className="composer__accessory">
                <div className="composer__flow-controls">
                  <label className="composer__flow">
                    <IconFlows className="composer__flow-icon" />
                    <span className="composer__flow-label">Flow</span>
                    <select
                      id="workflow"
                      className="composer__select"
                      value={workflow}
                      onChange={(event) => setWorkflow(event.target.value)}
                      disabled={workflowsLoading || !hasUsableFlow}
                    >
                      {workflowsLoading || !hasUsableFlow ? (
                        <option value="">
                          {workflowsLoading ? "Loading flows…" : "No flows available"}
                        </option>
                      ) : null}
                      {workflows.map((entry) => (
                        <option
                          key={entry.reference}
                          value={entry.reference}
                          disabled={!entry.valid}
                        >
                          {entry.name?.trim() || entry.reference}
                          {entry.valid ? "" : " (invalid)"}
                        </option>
                      ))}
                    </select>
                  </label>
                  {!workflowsLoading && !hasUsableFlow ? (
                    <Button
                      variant="secondary"
                      size="sm"
                      className="composer__new-flow"
                      onClick={() => onNewFlow(project)}
                    >
                      <IconPlus /> New flow
                    </Button>
                  ) : null}
                </div>

                <button
                  type="button"
                  className="composer__project"
                  onClick={onOpenProject}
                  aria-haspopup="dialog"
                  title={project}
                >
                  <IconFolder />
                  <span>{basename(project)}</span>
                </button>
              </div>

              <label className="visually-hidden" htmlFor="objective">
                Objective
              </label>
              <textarea
                id="objective"
                className="composer__input"
                value={objective}
                onChange={(event) => setObjective(event.target.value)}
                placeholder="What should this run accomplish?"
                rows={3}
                autoFocus
                aria-describedby="objective-hint"
              />

              <div className="composer__foot">
                <span id="objective-hint" className="composer__hint">
                  {!hasUsableFlow
                    ? "Create a flow to start a run."
                    : objective.trim()
                      ? selected
                        ? `${selected.nodes.length} ${selected.nodes.length === 1 ? "node" : "nodes"} · ${selected.edges.length} ${selected.edges.length === 1 ? "edge" : "edges"}`
                        : "Choose a flow"
                      : "An objective is required."}
                </span>
                <button
                  type="submit"
                  className="composer__submit"
                  disabled={!canStart}
                  aria-label="Start run"
                >
                  {pendingAction === "start" ? (
                    <span className="composer__spinner" aria-hidden="true" />
                  ) : (
                    <IconArrowUp />
                  )}
                  <span>Start run</span>
                </button>
              </div>
            </form>
            )}

            {invalid.length > 0 ? (
              <p className="chat-home__warning">
                {invalid.length}{" "}
                {invalid.length === 1 ? "flow cannot" : "flows cannot"} be used:{" "}
                {invalid
                  .map(
                    (entry) =>
                      `${entry.name?.trim() || entry.reference} — ${entry.problems?.[0] ?? "invalid"}`,
                  )
                  .join("; ")}
              </p>
            ) : null}

          </div>
        )}
      </div>
    </div>
  );
}
