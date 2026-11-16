import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import { NODE_STATE_LABEL, type VisualState } from "../../../graph/nodeState";
import type { NodeKind } from "../../../types/run";
import { StatusIcon } from "../../common/Status";

export interface WorkflowNodeData extends Record<string, unknown> {
  title: string;
  kind: NodeKind;
  state: VisualState;
  onSelect: (nodeId: string) => void;
}

interface NodeCardViewProps {
  title: string;
  kind: NodeKind;
  state: VisualState;
  onSelect: () => void;
}

export function NodeCardView({ title, kind, state, onSelect }: NodeCardViewProps) {
  const waiting = state === "waiting_answer" || state === "waiting_approval";
  const failed = state === "failed";

  return (
    <button
      type="button"
      className={`node-card node-card--${state}`}
      onClick={onSelect}
      aria-label={`${title}, ${NODE_STATE_LABEL[state]}`}
    >
      <span className="node-card__head">
        <StatusIcon state={state} />
        <span className="node-card__title">{title}</span>
      </span>
      <span className="node-card__meta">
        <span className="node-card__kind">{kind}</span>
        <span className={`node-card__state node-card__state--${state}`}>
          {NODE_STATE_LABEL[state]}
        </span>
      </span>
      {waiting ? (
        <span className="node-card__cta">
          {state === "waiting_approval" ? "Needs approval" : "Needs your answer"}
        </span>
      ) : null}
      {failed ? <span className="node-card__cta">View error</span> : null}
    </button>
  );
}

export function NodeCard({ id, data }: NodeProps<Node<WorkflowNodeData>>) {
  return (
    <div className="node-card-wrap">
      <Handle type="target" position={Position.Top} className="node-card__handle" />
      <NodeCardView
        title={data.title}
        kind={data.kind}
        state={data.state}
        onSelect={() => data.onSelect(id)}
      />
      <Handle type="source" position={Position.Bottom} className="node-card__handle" />
    </div>
  );
}
