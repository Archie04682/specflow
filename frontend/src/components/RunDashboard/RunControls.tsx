import type { ReactNode } from "react";
import type { RunStatus } from "../../types/run";
import type { ActionName } from "../../stores/runStore";
import { shortId } from "../../lib/format";
import { Button } from "../common/Button";

interface RunControlsProps {
  runStatus: RunStatus;
  failedNode?: { nodeId: string; title?: string };
  pendingAction: ActionName | null;
  /** A completed folder run whose changes are not in the project yet. */
  awaitingApply?: boolean;
  /** A folder run whose copy can be discarded (awaiting apply, failed, or cancelled). */
  discardable?: boolean;
  onRequestCancel: () => void;
  onResume: () => void;
  onRetry: (nodeId: string) => void;
  onApply?: () => void;
  onRequestDiscard?: () => void;
}

const CANCELLABLE: RunStatus[] = ["running", "waiting_approval", "waiting_answer"];

// The controls a run permits in its workspace header. Kept as one component so
// the header, the dialog, and their tests describe the same rule set.
export function RunControls({
  runStatus,
  failedNode,
  pendingAction,
  awaitingApply = false,
  discardable = false,
  onRequestCancel,
  onResume,
  onRetry,
  onApply,
  onRequestDiscard,
}: RunControlsProps) {
  const busy = pendingAction !== null;
  const canCancel = CANCELLABLE.includes(runStatus);
  const canResume = runStatus === "failed" || runStatus === "cancelled";

  const controls: ReactNode[] = [];
  if (canCancel) {
    controls.push(
      <Button key="cancel" variant="danger" size="sm" disabled={busy} onClick={onRequestCancel}>
        Cancel run
      </Button>,
    );
  }
  if (failedNode) {
    controls.push(
      <Button
        key="retry"
        variant="primary"
        size="sm"
        loading={pendingAction === "retry"}
        disabled={busy}
        onClick={() => onRetry(failedNode.nodeId)}
      >
        Retry {failedNode.title ?? shortId(failedNode.nodeId)}
      </Button>,
    );
  }
  if (awaitingApply) {
    controls.push(
      <Button key="apply" variant="primary" size="sm" loading={pendingAction === "apply"} disabled={busy} onClick={onApply}>
        Apply changes
      </Button>,
    );
  }
  if (awaitingApply || discardable) {
    controls.push(
      <Button key="discard" variant="secondary" size="sm" disabled={busy} onClick={onRequestDiscard}>
        Discard
      </Button>,
    );
  }
  if (canResume) {
    controls.push(
      <Button
        key="resume"
        variant={failedNode ? "secondary" : "primary"}
        size="sm"
        loading={pendingAction === "resume"}
        disabled={busy}
        onClick={onResume}
      >
        Resume run
      </Button>,
    );
  }

  return <>{controls}</>;
}
