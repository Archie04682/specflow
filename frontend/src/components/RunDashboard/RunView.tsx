import { useState } from "react";
import {
  useEntryError,
  useGraphError,
  usePendingAction,
  useRunError,
  useRunGraph,
  useRunLoading,
  useRunNotice,
  useRunRecord,
  useRunStore,
  useStreamStatus,
} from "../../stores/runStore";
import { useRunData } from "../../hooks/useRunData";
import { useRunActions } from "../../hooks/useRunActions";
import { formatObjective, shortId } from "../../lib/format";
import { RunStatusChip } from "../common/Status";
import { Banner } from "../common/Banner";
import { Button } from "../common/Button";
import { ConfirmDialog } from "../common/Modal";
import { Spinner } from "../common/Spinner";
import { EventStreamPanel } from "./EventStreamPanel/EventStreamPanel";
import { GraphPanel } from "./GraphPanel/GraphPanel";
import { InteractionPanel } from "./InteractionPanel/InteractionPanel";
import { RunControls } from "./RunControls";
import { RunFooter } from "./RunFooter";
import { RunSplit } from "./RunSplit";
import { ViewHeader } from "../Shell/ViewHeader";

interface RunViewProps {
  project: string;
  runId: string;
  onNewRun: () => void;
}

export function RunView({ project, runId, onNewRun }: RunViewProps) {
  const run = useRunRecord();
  const graph = useRunGraph();
  const graphError = useGraphError();
  const loading = useRunLoading();
  const error = useRunError();
  const entryError = useEntryError();
  const notice = useRunNotice();
  const streamStatus = useStreamStatus();
  const pendingAction = usePendingAction();
  const setError = useRunStore((state) => state.setError);
  const setEntryError = useRunStore((state) => state.setEntryError);
  const setNotice = useRunStore((state) => state.setNotice);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [discardOpen, setDiscardOpen] = useState(false);

  const { refresh, reconnect } = useRunData(project, runId);
  const actions = useRunActions({ project, runId, refresh, reconnect });

  if (!run) {
    return (
      <div className="workspace-view run-view">
        <ViewHeader
          title="Run"
          subtitle={<span className="mono">{shortId(runId)}</span>}
          actions={
            <Button variant="ghost" size="sm" onClick={onNewRun}>
              New chat
            </Button>
          }
        />
        <div className="workspace-fill centered-state">
          {error ? (
            <>
              <Banner
                tone="error"
                action={
                  <Button size="sm" onClick={() => void refresh()}>
                    Retry
                  </Button>
                }
              >
                {error}
              </Banner>
              <Button variant="secondary" onClick={onNewRun}>
                Back to new chat
              </Button>
            </>
          ) : (
            <Spinner label="Loading run" />
          )}
        </div>
      </div>
    );
  }

  const confirmCancel = async () => {
    await actions.cancel();
    setCancelOpen(false);
  };

  // A folder run's changes live in its copy until the operator applies them.
  // A failed or cancelled folder run holds that copy too: discarding is the
  // only way to release the project, so it must stay reachable.
  const folder = run.vcs === "folder";
  const awaitingApply =
    folder && run.status === "completed" && run.apply === "pending";
  const discardable =
    awaitingApply ||
    (folder && (run.status === "failed" || run.status === "cancelled"));
  const confirmDiscard = async () => {
    await actions.discard();
    setDiscardOpen(false);
  };

  const failedNode = run.nodes.find((node) => node.status === "failed");
  const workflowName = graph?.name ?? run.workflowId ?? "workflow";

  return (
    <div className="workspace-view run-view">
      <ViewHeader
        title={
          <span className="run-view__objective" title={run.objective}>
            {formatObjective(run.objective, 96)}
          </span>
        }
        subtitle={
          <>
            <RunStatusChip status={run.status} />
            <span>{workflowName}</span>
            <span className="view-header__sep" aria-hidden="true">
              ·
            </span>
            <span className="mono" title={run.id}>
              {shortId(run.id)}
            </span>
          </>
        }
        actions={
          <>
            <RunControls
              runStatus={run.status}
              failedNode={
                failedNode
                  ? { nodeId: failedNode.nodeId, title: failedNode.title }
                  : undefined
              }
              pendingAction={pendingAction}
              awaitingApply={awaitingApply}
              discardable={discardable}
              onApply={() => void actions.apply()}
              onRequestDiscard={() => setDiscardOpen(true)}
              onRequestCancel={() => setCancelOpen(true)}
              onResume={() => void actions.resume()}
              onRetry={(nodeId) => void actions.retry(nodeId, "forward")}
            />
            <Button variant="ghost" size="sm" onClick={onNewRun}>
              New chat
            </Button>
          </>
        }
      />

      <div className="run-banners">
        {awaitingApply ? (
          <Banner tone="info">
            This run worked on a copy of the folder. Its changes are not in the
            project yet: apply them, or discard the run.
          </Banner>
        ) : null}
        {entryError ? (
          <Banner tone="error" onDismiss={() => setEntryError(null)}>
            {entryError}
          </Banner>
        ) : null}
        {run.status === "failed" && run.error ? (
          <Banner tone="error">Run failed: {run.error}</Banner>
        ) : null}
        {error ? (
          <Banner
            tone="error"
            onDismiss={() => setError(null)}
            action={
              <Button size="sm" onClick={() => void refresh()}>
                Retry
              </Button>
            }
          >
            {error}
          </Banner>
        ) : null}
        {notice ? (
          <Banner tone="info" onDismiss={() => setNotice(null)}>
            {notice}
          </Banner>
        ) : null}
      </div>

      <RunSplit
        busy={loading}
        graph={
          <GraphPanel graph={graph} graphError={graphError} run={run} actions={actions} />
        }
        side={
          <div className="run-side">
            <EventStreamPanel key={run.id} />
            <InteractionPanel run={run} actions={actions} />
          </div>
        }
      />

      <RunFooter run={run} streamStatus={streamStatus} />

      <ConfirmDialog
        open={discardOpen}
        title="Discard this run's changes?"
        confirmLabel="Discard changes"
        cancelLabel="Keep them"
        danger
        busy={pendingAction === "discard"}
        onConfirm={() => void confirmDiscard()}
        onCancel={() => setDiscardOpen(false)}
      >
        The run's copy is removed and the project's files stay as they are.
      </ConfirmDialog>

      <ConfirmDialog
        open={cancelOpen}
        title="Cancel this run?"
        confirmLabel="Cancel run"
        cancelLabel="Keep running"
        danger
        busy={pendingAction === "cancel"}
        onConfirm={() => void confirmCancel()}
        onCancel={() => setCancelOpen(false)}
      >
        The running node is stopped and unfinished nodes are left unrun. You can resume the run
        later.
      </ConfirmDialog>
    </div>
  );
}
