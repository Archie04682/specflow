import type { ApprovalDecision } from "../../../api/runs";
import { formatClock, truncate } from "../../../lib/format";
import type { PendingInteraction } from "../../../types/run";
import { Button } from "../../common/Button";

interface ApprovalCardProps {
  pending: PendingInteraction;
  busy: boolean;
  onDecide: (decision: ApprovalDecision) => Promise<boolean>;
}

export function ApprovalCard({ pending, busy, onDecide }: ApprovalCardProps) {
  const attempt = pending.attempt ?? 0;

  return (
    <div className="interaction-card interaction-card--approval">
      <div className="interaction-card__head">
        <span className="eyebrow">Approval gate</span>
        <h3>{pending.title ?? pending.nodeId}</h3>
      </div>
      <p className="muted small">
        Node <span className="mono">{pending.nodeId}</span> · attempt {attempt}
        {pending.deadline ? ` · deadline ${formatClock(pending.deadline)}` : ""}
      </p>
      {pending.action ? (
        <pre className="code-block">{truncate(pending.action, 1600)}</pre>
      ) : null}
      <p className="field__hint">
        Rejecting ends the run cancelled; the node is not started.
      </p>
      <div className="interaction-card__actions">
        <Button
          variant="primary"
          loading={busy}
          disabled={busy}
          onClick={() => void onDecide("approve")}
        >
          Approve
        </Button>
        <Button variant="danger" disabled={busy} onClick={() => void onDecide("reject")}>
          Reject
        </Button>
      </div>
    </div>
  );
}
