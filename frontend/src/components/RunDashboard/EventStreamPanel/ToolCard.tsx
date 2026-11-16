import { Fragment } from "react";
import { formatMilliseconds } from "../../../lib/format";
import { describeTool, type ToolCallDescription } from "../../../lib/toolArguments";
import type { StreamItem } from "../../../projection/eventProjection";
import { Markdown } from "../../common/Markdown";
import { TerminalOutput } from "../../common/TerminalOutput";

type ToolItem = Extract<StreamItem, { kind: "tool" }>;

function renderInput(description: ToolCallDescription) {
  switch (description.kind) {
    case "command":
      return (
        <div className="tool-card__input">
          <div className="tool-card__command">
            <span className="tool-card__prompt" aria-hidden="true">
              $
            </span>
            <span>{description.command}</span>
          </div>
          {description.timeout !== undefined ? (
            <span className="tool-card__detail">timeout {description.timeout}s</span>
          ) : null}
        </div>
      );
    case "file":
      return (
        <div className="tool-card__input">
          <div className="tool-card__path">
            <span className="tool-card__path-name">{description.path}</span>
            {description.detail ? (
              <span className="tool-card__detail">{description.detail}</span>
            ) : null}
          </div>
        </div>
      );
    case "search":
      return (
        <div className="tool-card__input">
          <div className="tool-card__path">
            <span className="tool-card__path-name">{description.pattern}</span>
            {description.path ? (
              <span className="tool-card__detail">in {description.path}</span>
            ) : null}
          </div>
        </div>
      );
    case "question":
      return (
        <div className="tool-card__question">
          {description.title ? (
            <span className="tool-card__question-title">{description.title}</span>
          ) : null}
          {description.description ? <Markdown text={description.description} /> : null}
          {description.options.length > 0 ? (
            <ul className="tool-card__question-options">
              {description.options.map((option) => (
                <li key={option} className="tool-card__question-option">
                  {option}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      );
    case "generic":
      return (
        <dl className="tool-card__args">
          {description.entries.map((entry) => (
            <Fragment key={entry.label}>
              <dt className="tool-card__arg-label">{entry.label}</dt>
              <dd className="tool-card__arg-value">{entry.value}</dd>
            </Fragment>
          ))}
        </dl>
      );
    default:
      return null;
  }
}

/** One rendered tool call: what ran, what it touched, and what it produced. */
export function ToolCard({ item }: { item: ToolItem }) {
  const stateLabel =
    item.state === "running" ? "running" : item.state === "error" ? "failed" : "done";
  const duration = formatMilliseconds(item.durationMs);
  const description = describeTool(item.name, item.args);

  return (
    <div className={`tool-card tool-card--${item.state}`}>
      <div className="tool-card__head">
        <span className="tool-card__name mono">{item.name}</span>
        <span className={`tool-card__state tool-card__state--${item.state}`}>{stateLabel}</span>
        {duration ? <span className="muted small mono">{duration}</span> : null}
      </div>
      {renderInput(description)}
      {item.summary ? (
        <TerminalOutput
          text={item.summary}
          failed={item.state === "error"}
          label={`${item.name} output`}
        />
      ) : null}
      {item.files.length > 0 ? (
        <ul className="file-chips" aria-label="Files changed by this tool">
          {item.files.map((file) => (
            <li key={file} className="file-chip mono">
              {file}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
