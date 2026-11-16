import type { RunStatus } from "../../types/run";
import { NODE_STATE_LABEL, type VisualState } from "../../graph/nodeState";
import {
  IconCancelled,
  IconCheck,
  IconCircle,
  IconCross,
  IconDash,
  IconPause,
  IconPlay,
  IconQuestion,
} from "./icons";

export type StatusTone = "neutral" | "running" | "warning" | "success" | "danger" | "muted";

const RUN_STATUS_LABEL: Record<RunStatus, string> = {
  pending: "Pending",
  running: "Running",
  waiting_approval: "Waiting approval",
  waiting_answer: "Waiting answer",
  completed: "Completed",
  failed: "Failed",
  cancelled: "Cancelled",
};

export function runStatusLabel(status: RunStatus): string {
  return RUN_STATUS_LABEL[status];
}

export function runStatusTone(status: RunStatus): StatusTone {
  switch (status) {
    case "running":
      return "running";
    case "waiting_approval":
    case "waiting_answer":
      return "warning";
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "cancelled":
      return "muted";
    case "pending":
    default:
      return "neutral";
  }
}

export function nodeStateTone(state: VisualState): StatusTone {
  switch (state) {
    case "running":
      return "running";
    case "waiting_approval":
    case "waiting_answer":
      return "warning";
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "cancelled":
    case "skipped":
    case "not_run":
      return "muted";
    case "pending":
    default:
      return "neutral";
  }
}

export function RunStatusChip({ status }: { status: RunStatus }) {
  return (
    <span className={`chip chip--${runStatusTone(status)}`}>
      <StatusIcon state={status} />
      <span>{RUN_STATUS_LABEL[status]}</span>
    </span>
  );
}

export function NodeStateChip({ state }: { state: VisualState }) {
  return (
    <span className={`chip chip--${nodeStateTone(state)}`}>
      <StatusIcon state={state} />
      <span>{NODE_STATE_LABEL[state]}</span>
    </span>
  );
}

export function StatusIcon({ state }: { state: VisualState }) {
  switch (state) {
    case "running":
      return <IconPlay />;
    case "waiting_approval":
      return <IconPause />;
    case "waiting_answer":
      return <IconQuestion />;
    case "completed":
      return <IconCheck />;
    case "failed":
      return <IconCross />;
    case "cancelled":
      return <IconCancelled />;
    case "skipped":
    case "not_run":
      return <IconDash />;
    case "pending":
    default:
      return <IconCircle />;
  }
}
