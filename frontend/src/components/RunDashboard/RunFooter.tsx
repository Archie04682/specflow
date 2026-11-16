import { useEffect, useState } from "react";
import { formatClock, formatCost, formatDuration, formatTokens } from "../../lib/format";
import { isTerminalRun, totalUsage, type Run } from "../../types/run";
import { useLastStreamAt, type StreamStatus } from "../../stores/runStore";
import { CONNECTION_LABEL, connectionTone } from "./connection";

interface RunFooterProps {
  run: Run;
  streamStatus: StreamStatus;
}

// The run's telemetry sits in a thin status bar under the workspace instead of
// a page footer; it is the same read-only summary the run always exposed.
export function RunFooter({ run, streamStatus }: RunFooterProps) {
  const [now, setNow] = useState(() => Date.now());
  const lastEventAt = useLastStreamAt();

  useEffect(() => {
    if (isTerminalRun(run.status)) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [run.status]);

  const usage = totalUsage(run.nodes);
  const completed = run.nodes.filter((node) => node.status === "completed").length;
  const tone = connectionTone(streamStatus);

  return (
    <footer className="run-statusbar" aria-label="Run summary">
      <span className="run-statusbar__metric">
        <span className="run-statusbar__label">Duration</span>
        <span className="run-statusbar__value mono">
          {formatDuration(run.startedAt, run.finishedAt, now)}
        </span>
      </span>
      <span className="run-statusbar__metric">
        <span className="run-statusbar__label">Nodes</span>
        <span className="run-statusbar__value mono">
          {completed}/{run.nodes.length}
        </span>
      </span>
      <span className="run-statusbar__metric">
        <span className="run-statusbar__label">Tokens</span>
        <span className="run-statusbar__value mono">{formatTokens(usage.tokens)}</span>
      </span>
      <span className="run-statusbar__metric">
        <span className="run-statusbar__label">Cost</span>
        <span className="run-statusbar__value mono">{formatCost(usage.cost)}</span>
      </span>
      <span className="run-statusbar__metric">
        <span className="run-statusbar__label">Last event</span>
        <span className="run-statusbar__value mono">
          {formatClock(lastEventAt)}
        </span>
      </span>
      <span className={`connection connection--${tone} run-statusbar__connection`}>
        <span className="connection__dot" aria-hidden="true" />
        <span>{CONNECTION_LABEL[streamStatus]}</span>
      </span>
    </footer>
  );
}
