import { useState, type FormEvent } from "react";
import type { AnswerPayload } from "../../../api/runs";
import { formatDeadline } from "../../../lib/format";
import type { AskShape } from "../../../types/events";
import type { PendingInteraction } from "../../../types/run";
import { Button } from "../../common/Button";
import { Markdown } from "../../common/Markdown";

type AskKind = "input" | "select" | "confirm" | "editor";

interface AskCardProps {
  question: PendingInteraction;
  ask: AskShape;
  busy: boolean;
  onSubmit: (payload: AnswerPayload) => Promise<boolean>;
}

export function AskCard({ question, ask, busy, onSubmit }: AskCardProps) {
  const kind: AskKind =
    ask.kind === "select" || ask.kind === "confirm" || ask.kind === "editor"
      ? ask.kind
      : "input";
  const [value, setValue] = useState(
    kind === "editor" ? (ask.prefill ?? "") : (ask.default ?? ""),
  );
  const options = ask.options ?? [];

  const cancel = () => {
    void onSubmit({ questionId: question.id, cancelled: true });
  };

  const submitValue = async (event: FormEvent) => {
    event.preventDefault();
    if (!value.trim()) return;
    await onSubmit({ questionId: question.id, value });
  };

  return (
    <div className="interaction-card">
      <div className="interaction-card__head">
        <span className="eyebrow">Question</span>
        <h3>{ask.title ?? "The run needs an answer"}</h3>
      </div>
      {ask.message ? (
        <Markdown text={ask.message} className="interaction-card__message" />
      ) : null}
      <p className="muted small">
        Attempt {question.attempt ?? ask.attempt ?? 0} ·{" "}
        {formatDeadline(ask.deadline ?? question.deadline)}
      </p>

      {kind === "confirm" ? (
        <div className="interaction-card__actions">
          <Button
            variant="primary"
            loading={busy}
            disabled={busy}
            onClick={() => void onSubmit({ questionId: question.id, confirmed: true })}
          >
            Yes
          </Button>
          <Button
            variant="secondary"
            disabled={busy}
            onClick={() => void onSubmit({ questionId: question.id, confirmed: false })}
          >
            No
          </Button>
          <Button variant="ghost" disabled={busy} onClick={cancel}>
            Cancel question
          </Button>
        </div>
      ) : kind === "select" ? (
        <div className="interaction-card__actions interaction-card__actions--column">
          {options.map((option) => (
            <Button
              key={option}
              className="interaction-card__option"
              variant="secondary"
              disabled={busy}
              onClick={() => void onSubmit({ questionId: question.id, value: option })}
            >
              {option}
            </Button>
          ))}
          {options.length === 0 ? (
            <p className="muted small">This question offered no options.</p>
          ) : null}
          <Button variant="ghost" disabled={busy} onClick={cancel}>
            Cancel question
          </Button>
        </div>
      ) : (
        <form className="form" onSubmit={submitValue}>
          <label className="field__label" htmlFor={`answer-${question.id}`}>
            Answer
          </label>
          {kind === "editor" ? (
            <textarea
              id={`answer-${question.id}`}
              className="input"
              rows={5}
              value={value}
              onChange={(event) => setValue(event.target.value)}
              placeholder={ask.placeholder}
              disabled={busy}
            />
          ) : (
            <input
              id={`answer-${question.id}`}
              className="input"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              placeholder={ask.placeholder ?? "Type an answer"}
              disabled={busy}
            />
          )}
          <div className="interaction-card__actions">
            <Button
              type="submit"
              variant="primary"
              loading={busy}
              disabled={busy || !value.trim()}
            >
              Send answer
            </Button>
            <Button variant="ghost" disabled={busy} onClick={cancel}>
              Cancel question
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}
