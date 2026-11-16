import { useEffect, useRef } from "react";
import { deriveNodeState } from "../../../graph/nodeState";
import { formatCost, formatDateTime, formatTokens, shortId } from "../../../lib/format";
import type { GraphNode } from "../../../types/graph";
import type { NodeRun, Run } from "../../../types/run";
import type { RetryMode } from "../../../api/runs";
import { Button } from "../../common/Button";
import { IconCross } from "../../common/icons";
import { TerminalOutput } from "../../common/TerminalOutput";
import { NodeStateChip } from "../../common/Status";

interface NodeDetailsProps {
  node: NodeRun;
  graphNode: GraphNode;
  run: Run;
  busy: boolean;
  onClose: () => void;
  onRetry: (mode: RetryMode) => void;
}

export function NodeDetails({
  node,
  graphNode,
  run,
  busy,
  onClose,
  onRetry,
}: NodeDetailsProps) {
  const closeRef = useRef<HTMLButtonElement>(null);
  const state = deriveNodeState(node, run);
  const title = node.title ?? graphNode.title ?? node.nodeId;
  // A successful command's exit code is 0, which the record omits. A completed
  // command node therefore reads as exit 0; anything that never ran stays "—".
  const commandExitCode =
    node.exitCode ?? (node.status === "completed" ? 0 : undefined);

  useEffect(() => {
    closeRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  return (
    <aside className="drawer" aria-label={`${title} details`}>
      <header className="drawer__header">
        <div>
          <p className="eyebrow">Node</p>
          <h3 className="drawer__title">{title}</h3>
        </div>
        <button
          ref={closeRef}
          type="button"
          className="icon-button"
          aria-label="Close node details"
          onClick={onClose}
        >
          <IconCross />
        </button>
      </header>

      <div className="drawer__body">
        <NodeStateChip state={state} />

        <dl className="detail-grid">
          <div>
            <dt>Kind</dt>
            <dd className="mono">{node.kind}</dd>
          </div>
          <div>
            <dt>Attempts</dt>
            <dd className="mono">{node.attempts ?? 0}</dd>
          </div>
          <div>
            <dt>Started</dt>
            <dd>{formatDateTime(node.startedAt)}</dd>
          </div>
          <div>
            <dt>Ended</dt>
            <dd>{formatDateTime(node.endedAt)}</dd>
          </div>
          <div>
            <dt>Tokens</dt>
            <dd className="mono">{formatTokens(node.usage.totalTokens ?? 0)}</dd>
          </div>
          <div>
            <dt>Cost</dt>
            <dd className="mono">{formatCost(node.usage.costUsd ?? 0)}</dd>
          </div>
          {node.checkpoint ? (
            <div>
              <dt>Checkpoint</dt>
              <dd className="mono" title={node.checkpoint}>
                {shortId(node.checkpoint, 12)}
              </dd>
            </div>
          ) : null}
          {graphNode.requireApproval ? (
            <div>
              <dt>Gate</dt>
              <dd>Approval required before every attempt</dd>
            </div>
          ) : null}
          {node.failureClass ? (
            <div>
              <dt>Failure class</dt>
              <dd className="mono">{node.failureClass}</dd>
            </div>
          ) : null}
          {typeof node.exitCode === "number" && node.exitCode !== 0 ? (
            <div>
              <dt>Exit code</dt>
              <dd className="mono">{node.exitCode}</dd>
            </div>
          ) : null}
        </dl>

        {node.kind === "command" &&
        (node.command !== undefined ||
          node.output !== undefined ||
          typeof node.exitCode === "number") ? (
          <div className="drawer__command">
            <h4>Command</h4>
            <pre className="code-block">{node.command ?? "(command was not recorded)"}</pre>
            <div className="drawer__command-meta mono">exit {commandExitCode ?? "—"}</div>
            {node.output ? (
              <TerminalOutput
                text={node.output}
                failed={node.status === "failed"}
                label="Command output"
              />
            ) : null}
          </div>
        ) : null}

        {node.error ? (
          <div className="drawer__error">
            <h4>Error</h4>
            <pre className="error-output">{node.error}</pre>
          </div>
        ) : null}

        {node.status === "failed" ? (
          <div className="drawer__actions">
            <Button
              variant="primary"
              size="sm"
              loading={busy}
              disabled={busy}
              onClick={() => onRetry("forward")}
            >
              Retry forward
            </Button>
            <Button
              variant="secondary"
              size="sm"
              loading={busy}
              disabled={busy}
              onClick={() => onRetry("clean")}
            >
              Retry clean
            </Button>
            <p className="field__hint">
              Forward keeps the failed attempt's changes. Clean restores the node's checkpoint
              first.
            </p>
          </div>
        ) : null}
      </div>
    </aside>
  );
}
