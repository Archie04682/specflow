import { formatTokens } from "../../../lib/format";
import { IconChevronDown, IconChevronRight } from "../../common/icons";
import { Markdown } from "../../common/Markdown";

interface ThinkingBlockProps {
  id: number;
  text: string;
  complete: boolean;
  expanded: boolean;
  onToggle: () => void;
}

/**
 * Reasoning is collapsed by default. A single run can emit tens of thousands
 * of characters of thinking, so the row reports what is there (size and
 * streaming state) and leaves the reading to the operator.
 */
export function ThinkingBlock({
  id,
  text,
  complete,
  expanded,
  onToggle,
}: ThinkingBlockProps) {
  const bodyId = `thinking-body-${id}`;

  return (
    <div className={complete ? "thinking" : "thinking thinking--streaming"}>
      <button
        type="button"
        className="thinking__head"
        aria-expanded={expanded}
        aria-controls={expanded ? bodyId : undefined}
        onClick={onToggle}
      >
        {expanded ? (
          <IconChevronDown className="thinking__chevron" />
        ) : (
          <IconChevronRight className="thinking__chevron" />
        )}
        <span className="thinking__label">Thinking</span>
        <span className="thinking__size">{formatTokens(text.length)} chars</span>
        {complete ? null : <span className="thinking__live">streaming</span>}
      </button>
      {expanded ? (
        <div className="thinking__body" id={bodyId}>
          {complete ? (
            <Markdown text={text} />
          ) : (
            <p className="thinking__plain">{text || "…"}</p>
          )}
        </div>
      ) : null}
    </div>
  );
}
