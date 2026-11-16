import { formatMilliseconds } from "../../../lib/format";
import type { StreamItem } from "../../../projection/eventProjection";
import { TerminalOutput } from "../../common/TerminalOutput";

type CommandItem = Extract<StreamItem, { kind: "command" }>;

/** A workflow command node: the gate's command line and its captured output. */
export function CommandCard({ item }: { item: CommandItem }) {
  const stateLabel =
    item.state === "running" ? "running" : item.state === "error" ? "failed" : "done";
  const duration = formatMilliseconds(item.durationMs);

  return (
    <div className={`command-card command-card--${item.state}`}>
      <div className="command-card__head">
        <div className="command-card__line">
          <span className="tool-card__prompt" aria-hidden="true">
            $
          </span>
          <span>{item.command}</span>
        </div>
        <div className="command-card__meta">
          <span>{stateLabel}</span>
          {item.exitCode !== undefined ? <span>exit {item.exitCode}</span> : null}
          {duration ? <span>{duration}</span> : null}
        </div>
      </div>
      {item.output ? (
        <TerminalOutput
          text={item.output}
          failed={item.state === "error"}
          label="Command output"
        />
      ) : null}
    </div>
  );
}
