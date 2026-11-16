import { memo, type ReactNode } from "react";
import { formatClock, truncate } from "../../../lib/format";
import type { StreamItem } from "../../../projection/eventProjection";
import { Markdown } from "../../common/Markdown";
import { IconCheck, IconCross, IconDash, IconPause, IconQuestion } from "../../common/icons";
import { CommandCard } from "./CommandCard";
import { ThinkingBlock } from "./ThinkingBlock";
import { ToolCard } from "./ToolCard";

function text(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function count(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function LifecycleView({ eventType, data }: { eventType: string; data: Record<string, unknown> }) {
  switch (eventType) {
    case "run.started": {
      const workflow = text(data.workflowId);
      return (
        <div className="event-banner event-banner--info">
          <span>Run started</span>
          {workflow ? <span className="mono"> · {workflow}</span> : null}
        </div>
      );
    }
    case "run.completed":
      return (
        <div className="event-banner event-banner--success">
          <IconCheck />
          <span>{text(data.summary) ?? "Run completed"}</span>
        </div>
      );
    case "run.failed":
      return (
        <div className="event-banner event-banner--danger">
          <IconCross />
          <span>Run failed: {text(data.error) ?? "unknown error"}</span>
        </div>
      );
    case "run.cancelled":
      return (
        <div className="event-banner event-banner--muted">
          <IconDash />
          <span>Run cancelled</span>
        </div>
      );
    case "change.resolved":
      return (
        <div className="event-banner event-banner--info">
          <span>Change resolved</span>
          {text(data.change) ? <span className="mono"> · {text(data.change)}</span> : null}
        </div>
      );
    default:
      return <div className="event-banner">{eventType}</div>;
  }
}

function NodeView({
  eventType,
  data,
  nodeId,
}: {
  eventType: string;
  data: Record<string, unknown>;
  nodeId?: string;
}) {
  const title = text(data.title) ?? text(data.node) ?? nodeId ?? "node";
  const attempt = count(data.attempt);
  switch (eventType) {
    case "node.started":
      return (
        <div className="node-marker">
          <span className="node-marker__icon node-marker__icon--running" aria-hidden="true" />
          <span className="node-marker__title">{title}</span>
          {attempt ? <span className="muted small">attempt {attempt}</span> : null}
        </div>
      );
    case "node.completed":
      return (
        <div className="node-marker node-marker--success">
          <IconCheck />
          <span className="node-marker__title">{title} completed</span>
          {text(data.change) ? (
            <span className="muted small">change {text(data.change)}</span>
          ) : null}
        </div>
      );
    case "node.failed":
    case "node.postcondition_failed":
      return (
        <div className="node-marker node-marker--danger">
          <IconCross />
          <div>
            <span className="node-marker__title">
              {title} {eventType === "node.postcondition_failed" ? "did not satisfy its postcondition" : "failed"}
            </span>
            {text(data.error) ? <p className="event-error-text">{text(data.error)}</p> : null}
          </div>
        </div>
      );
    case "node.skipped":
      return (
        <div className="node-marker node-marker--muted">
          <IconDash />
          <span className="node-marker__title">{title} skipped</span>
        </div>
      );
    default:
      return <div className="node-marker">{eventType}</div>;
  }
}

function InteractionView({
  eventType,
  data,
}: {
  eventType: string;
  data: Record<string, unknown>;
}) {
  const options = Array.isArray(data.options)
    ? data.options.filter((option): option is string => typeof option === "string")
    : [];

  switch (eventType) {
    case "question.asked": {
      const message = text(data.message);
      return (
        <div className="event-card event-card--question">
          <div className="event-card__head">
            <IconQuestion />
            <span className="event-card__label">Question</span>
          </div>
          {text(data.title) ? <p className="event-card__title">{text(data.title)}</p> : null}
          {message ? <Markdown text={message} /> : null}
          {options.length > 0 ? (
            <ul className="option-list">
              {options.map((option) => (
                <li key={option}>{option}</li>
              ))}
            </ul>
          ) : null}
          <p className="muted small">Answer in the interaction panel.</p>
        </div>
      );
    }
    case "question.answered": {
      const value = text(data.value);
      return (
        <div className="event-card">
          <span className="event-card__label">Question answered</span>
          {value ? <Markdown text={value} /> : null}
          {typeof data.confirmed === "boolean" ? (
            <p className="mono">{data.confirmed ? "confirmed: yes" : "confirmed: no"}</p>
          ) : null}
        </div>
      );
    }
    case "question.timed_out":
      return <div className="event-card event-card--warning">Question timed out.</div>;
    case "question.cancelled":
      return <div className="event-card">Question cancelled.</div>;
    case "injection.received": {
      const injected = text(data.text);
      return (
        <div className="event-card event-card--injection">
          <span className="event-card__label">Clarification sent</span>
          {injected ? <Markdown text={injected} /> : null}
        </div>
      );
    }
    case "injection.acknowledged":
      return (
        <div className="event-card">
          <span className="event-card__label">
            Clarification {text(data.disposition) ?? "delivered"}
          </span>
        </div>
      );
    case "injection.rejected":
      return (
        <div className="event-card event-card--danger">
          <span className="event-card__label">Clarification rejected</span>
          <p>{text(data.error) ?? text(data.reason) ?? "not accepted"}</p>
        </div>
      );
    case "approval.required": {
      const action = text(data.action);
      return (
        <div className="event-card event-card--approval">
          <div className="event-card__head">
            <IconPause />
            <span className="event-card__label">Approval required</span>
          </div>
          {text(data.title) ? <p className="event-card__title">{text(data.title)}</p> : null}
          {action ? <pre className="code-block">{truncate(action, 1200)}</pre> : null}
          <p className="muted small">Decide in the interaction panel.</p>
        </div>
      );
    }
    case "approval.granted":
      return <div className="event-card event-card--success">Approval granted.</div>;
    case "approval.rejected":
      return (
        <div className="event-card event-card--danger">Approval rejected; the run was cancelled.</div>
      );
    case "approval.timed_out":
      return <div className="event-card event-card--danger">Approval timed out.</div>;
    default:
      return <div className="event-card">{eventType}</div>;
  }
}

// A row is memoized because a delta burst replaces the items array while only
// one item changes; the rest keep their identity and skip rendering.
export const EventItem = memo(function EventItem({
  item,
  expanded = false,
  onToggleThinking,
}: {
  item: StreamItem;
  expanded?: boolean;
  onToggleThinking?: (id: number) => void;
}) {
  let body: ReactNode;

  switch (item.kind) {
    case "lifecycle":
      body = <LifecycleView eventType={item.eventType} data={item.data} />;
      break;
    case "node":
      body = <NodeView eventType={item.eventType} data={item.data} nodeId={item.nodeId} />;
      break;
    case "interaction":
      body = <InteractionView eventType={item.eventType} data={item.data} />;
      break;
    case "thinking":
      body = (
        <ThinkingBlock
          id={item.id}
          text={item.text}
          complete={item.complete}
          expanded={expanded}
          onToggle={() => onToggleThinking?.(item.id)}
        />
      );
      break;
    case "message":
      body = (
        <div className={`event-text${item.complete ? "" : " event-text--streaming"}`}>
          <span className="event-text__label">Assistant</span>
          {item.complete ? (
            <Markdown text={item.text} />
          ) : (
            <p className="event-text__plain">{item.text}</p>
          )}
        </div>
      );
      break;
    case "tool":
      body = <ToolCard item={item} />;
      break;
    case "command":
      body = <CommandCard item={item} />;
      break;
    case "file":
      body = (
        <div className="file-row">
          <span className="event-card__label">File touched</span>
          <span className="mono">{item.path}</span>
          {item.tool ? <span className="muted small">via {item.tool}</span> : null}
        </div>
      );
      break;
    case "notice":
      body = (
        <div className={`event-card event-card--${item.level === "error" ? "danger" : item.level === "warning" ? "warning" : "info"}`}>
          <span className="event-card__label">Notice</span>
          <Markdown text={item.message} />
        </div>
      );
      break;
    case "status":
      body = <p className="event-status">{item.text}</p>;
      break;
    case "unknown":
      body = (
        <div className="event-card event-card--unknown">
          Unrecognized event kind <code className="mono">{item.kindName}</code>
        </div>
      );
      break;
    default:
      body = null;
  }

  return (
    <article className={`event-item event-item--${item.kind}`}>
      <time className="event-item__time mono" dateTime={item.at}>
        {formatClock(item.at)}
      </time>
      <div className="event-item__body">
        {item.nodeId ? (
          <span className="event-item__node mono" title={item.nodeId}>
            {item.nodeId}
          </span>
        ) : null}
        {body}
      </div>
    </article>
  );
});
