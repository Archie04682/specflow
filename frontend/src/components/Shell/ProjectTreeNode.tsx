import { useMemo } from "react";
import { useRuns } from "../../hooks/useRuns";
import { useWorkflows } from "../../hooks/useWorkflows";
import { formatObjective, formatRelativeTime, shortId } from "../../lib/format";
import type { ProjectLabel } from "../../lib/projects";
import { isTerminalRun, type Run } from "../../types/run";
import type { WorkflowSummary } from "../../types/graph";
import { Button } from "../common/Button";
import { IconFlows, IconPlus } from "../common/icons";
import { runStatusLabel, runStatusTone } from "../common/Status";

interface ProjectTreeNodeProps {
  label: ProjectLabel;
  active: boolean;
  expanded: boolean;
  currentRunId?: string;
  currentReference?: string;
  liveRun: Run | null;
  onToggle: () => void;
  onOpenRun: (runId: string) => void;
  onNewChat: () => void;
  onOpenFlow: (reference: string) => void;
  onNewFlow: () => void;
}

// A project is the tree's first level; its Chats and Flows leaves load only
// while it is expanded, so a collapsed project costs nothing.
export function ProjectTreeNode({
  label,
  active,
  expanded,
  currentRunId,
  currentReference,
  liveRun,
  onToggle,
  onOpenRun,
  onNewChat,
  onOpenFlow,
  onNewFlow,
}: ProjectTreeNodeProps) {
  const projectTitle = label.suffix
    ? `${label.name} (${label.suffix})`
    : label.name;

  return (
    <div className={`tree-project${active ? " tree-project--active" : ""}`}>
      <button
        type="button"
        className="tree-project__row"
        aria-expanded={expanded}
        aria-label={projectTitle}
        title={label.path}
        onClick={onToggle}
      >
        <svg
          className={`tree-project__chevron${expanded ? " tree-project__chevron--open" : ""}`}
          viewBox="0 0 16 16"
          width="12"
          height="12"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M6 3.6 10.4 8 6 12.4" />
        </svg>
        <svg
          className="tree-project__icon"
          viewBox="0 0 16 16"
          width="15"
          height="15"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M2.3 4.6a1.3 1.3 0 0 1 1.3-1.3h2.3l1.3 1.5h5.2a1.3 1.3 0 0 1 1.3 1.3v5.6a1.3 1.3 0 0 1-1.3 1.3H3.6a1.3 1.3 0 0 1-1.3-1.3Z" />
        </svg>
        <span className="tree-project__name">{label.name}</span>
        {label.suffix ? (
          <span className="tree-project__suffix">{" · "}{label.suffix}</span>
        ) : null}
      </button>

      {expanded ? (
        <ProjectNodeLeaves
          project={label.path}
          projectTitle={projectTitle}
          currentRunId={currentRunId}
          currentReference={currentReference}
          liveRun={liveRun}
          onOpenRun={onOpenRun}
          onNewChat={onNewChat}
          onOpenFlow={onOpenFlow}
          onNewFlow={onNewFlow}
        />
      ) : null}
    </div>
  );
}

interface ProjectNodeLeavesProps {
  project: string;
  projectTitle: string;
  currentRunId?: string;
  currentReference?: string;
  liveRun: Run | null;
  onOpenRun: (runId: string) => void;
  onNewChat: () => void;
  onOpenFlow: (reference: string) => void;
  onNewFlow: () => void;
}

function ProjectNodeLeaves({
  project,
  projectTitle,
  currentRunId,
  currentReference,
  liveRun,
  onOpenRun,
  onNewChat,
  onOpenFlow,
  onNewFlow,
}: ProjectNodeLeavesProps) {
  const runsState = useRuns(project);
  const flowsState = useWorkflows(project);

  const runs = useMemo(() => {
    const byId = new Map(runsState.runs.map((run) => [run.id, run]));
    if (liveRun && liveRun.project === project) byId.set(liveRun.id, liveRun);
    const list = [...byId.values()];
    const newestFirst = (left: Run, right: Run) =>
      Date.parse(right.startedAt) - Date.parse(left.startedAt);
    return [
      ...list.filter((run) => !isTerminalRun(run.status)).sort(newestFirst),
      ...list.filter((run) => isTerminalRun(run.status)).sort(newestFirst),
    ];
  }, [runsState.runs, liveRun, project]);

  return (
    <div className="tree-children">
      <section className="tree-leaf" aria-label={`Chats in ${projectTitle}`}>
        <div className="tree-leaf__head">
          <span className="tree-leaf__label">Chats</span>
          {runsState.runs.length > 0 ? (
            <span className="tree-leaf__count" aria-hidden="true">
              {runsState.runs.length}
            </span>
          ) : null}
          <button
            type="button"
            className="icon-button icon-button--sm tree-leaf__action"
            aria-label={`New chat in ${projectTitle}`}
            title={`New chat in ${projectTitle}`}
            onClick={onNewChat}
          >
            <IconPlus />
          </button>
        </div>

        {runsState.error ? (
          <div className="tree-leaf__error" role="alert">
            <span>{runsState.error}</span>
            <Button variant="ghost" size="sm" onClick={() => void runsState.reload()}>
              Retry
            </Button>
          </div>
        ) : runsState.loading && runs.length === 0 ? (
          <div className="tree-leaf__loading" role="status" aria-live="polite">
            <span className="visually-hidden">Loading chats in {projectTitle}</span>
            <span className="catalog-skeleton catalog-skeleton--sm" aria-hidden="true" />
          </div>
        ) : runs.length === 0 ? (
          <p className="tree-leaf__empty">No chats yet</p>
        ) : (
          <ul className="catalog-list">
            {runs.map((run) => (
              <ChatRow
                key={run.id}
                run={run}
                current={run.id === currentRunId}
                onOpen={onOpenRun}
              />
            ))}
          </ul>
        )}
      </section>

      <section className="tree-leaf" aria-label={`Flows in ${projectTitle}`}>
        <div className="tree-leaf__head">
          <span className="tree-leaf__label">Flows</span>
          {flowsState.workflows.length > 0 ? (
            <span className="tree-leaf__count" aria-hidden="true">
              {flowsState.workflows.length}
            </span>
          ) : null}
          <button
            type="button"
            className="icon-button icon-button--sm tree-leaf__action"
            aria-label={`New flow in ${projectTitle}`}
            title={`New flow in ${projectTitle}`}
            onClick={onNewFlow}
          >
            <IconPlus />
          </button>
        </div>

        {flowsState.error ? (
          <div className="tree-leaf__error" role="alert">
            <span>{flowsState.error}</span>
            <Button variant="ghost" size="sm" onClick={() => void flowsState.reload()}>
              Retry
            </Button>
          </div>
        ) : flowsState.loading && flowsState.workflows.length === 0 ? (
          <div className="tree-leaf__loading" role="status" aria-live="polite">
            <span className="visually-hidden">Loading flows in {projectTitle}</span>
            <span className="catalog-skeleton catalog-skeleton--sm" aria-hidden="true" />
          </div>
        ) : flowsState.workflows.length === 0 ? (
          <p className="tree-leaf__empty">No flows yet</p>
        ) : (
          <ul className="catalog-list">
            {flowsState.workflows.map((workflow) => (
              <FlowRow
                key={workflow.reference}
                workflow={workflow}
                current={workflow.reference === currentReference}
                onOpen={onOpenFlow}
              />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

interface ChatRowProps {
  run: Run;
  current: boolean;
  onOpen: (runId: string) => void;
}

function ChatRow({ run, current, onOpen }: ChatRowProps) {
  const live = !isTerminalRun(run.status);
  const completed = run.nodes.filter((node) => node.status === "completed").length;
  const total = run.nodes.length;

  return (
    <li>
      <button
        type="button"
        className={`catalog-row${current ? " catalog-row--current" : ""}`}
        aria-current={current || undefined}
        aria-label={`Open chat ${shortId(run.id)}: ${runStatusLabel(run.status)} — ${formatObjective(run.objective, 70)}`}
        title={`${formatObjective(run.objective, 120)} — ${run.workflowId ?? "workflow"}`}
        onClick={() => onOpen(run.id)}
      >
        <span
          className={`catalog-row__dot catalog-row__dot--${runStatusTone(run.status)}`}
          aria-hidden="true"
        />
        <span className="catalog-row__main">
          <span className="catalog-row__title">
            {formatObjective(run.objective, 80)}
          </span>
          <span className="catalog-row__meta">
            <span className="catalog-row__state">{runStatusLabel(run.status)}</span>
            <span className="catalog-row__sep" aria-hidden="true">
              ·
            </span>
            <span>{formatRelativeTime(run.startedAt)}</span>
          </span>
        </span>
        {live && total > 0 ? (
          <span className="catalog-row__progress mono" aria-hidden="true">
            {completed}/{total}
          </span>
        ) : null}
      </button>
    </li>
  );
}

interface FlowRowProps {
  workflow: WorkflowSummary;
  current: boolean;
  onOpen: (reference: string) => void;
}

function FlowRow({ workflow, current, onOpen }: FlowRowProps) {
  const name = workflow.name?.trim() || workflow.reference;
  return (
    <li>
      <button
        type="button"
        className={`catalog-row catalog-row--flow${current ? " catalog-row--current" : ""}`}
        aria-current={current || undefined}
        aria-label={`Open flow ${name}${workflow.valid ? "" : " (invalid)"}`}
        title={
          workflow.valid ? undefined : workflow.problems?.join("\n") ?? "Invalid workflow"
        }
        onClick={() => onOpen(workflow.reference)}
      >
        <span
          className={`catalog-row__flow-icon${workflow.valid ? "" : " catalog-row__flow-icon--invalid"}`}
          aria-hidden="true"
        >
          <IconFlows />
        </span>
        <span className="catalog-row__main">
          <span className="catalog-row__title">{name}</span>
          <span className="catalog-row__meta">
            {workflow.valid ? (
              <>
                <span>
                  {workflow.nodes.length}{" "}
                  {workflow.nodes.length === 1 ? "node" : "nodes"}
                </span>
                <span className="catalog-row__sep" aria-hidden="true">
                  ·
                </span>
                <span>
                  {workflow.edges.length}{" "}
                  {workflow.edges.length === 1 ? "edge" : "edges"}
                </span>
              </>
            ) : (
              <span className="catalog-row__invalid">Invalid flow</span>
            )}
          </span>
        </span>
      </button>
    </li>
  );
}
