import { useState } from "react";

interface TerminalOutputProps {
  text: string;
  failed?: boolean;
  label?: string;
}

// Terminal-style output stays preformatted. Long output is collapsed to its
// first lines so one verbose command cannot push the activity feed off screen;
// the toggle reveals everything, so nothing is clipped without a way to read it.
const COLLAPSE_LINES = 12;
const COLLAPSE_CHARS = 1200;

function firstLines(text: string): string {
  const kept = text.split("\n").slice(0, COLLAPSE_LINES).join("\n");
  return kept.length > COLLAPSE_CHARS ? kept.slice(0, COLLAPSE_CHARS) : kept;
}

export function TerminalOutput({ text, failed = false, label }: TerminalOutputProps) {
  const [expanded, setExpanded] = useState(false);
  const lineCount = text.length === 0 ? 1 : text.split("\n").length;
  const collapsible = lineCount > COLLAPSE_LINES || text.length > COLLAPSE_CHARS;
  const body = expanded || !collapsible ? text : firstLines(text);

  return (
    <div className="terminal-wrap">
      <pre className={failed ? "terminal terminal--failed" : "terminal"} aria-label={label}>
        {body || "…"}
      </pre>
      {collapsible ? (
        <button
          type="button"
          className="terminal__toggle"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? "Show less" : `Show all (${lineCount} lines)`}
        </button>
      ) : null}
    </div>
  );
}
