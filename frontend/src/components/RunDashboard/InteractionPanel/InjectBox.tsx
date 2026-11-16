import { useState, type FormEvent, type KeyboardEvent } from "react";
import { Button } from "../../common/Button";

interface InjectBoxProps {
  busy: boolean;
  onSend: (text: string) => Promise<boolean>;
}

export function InjectBox({ busy, onSend }: InjectBoxProps) {
  const [text, setText] = useState("");
  const [refused, setRefused] = useState(false);

  const submit = async (event?: FormEvent) => {
    event?.preventDefault();
    const trimmed = text.trim();
    if (!trimmed) return;
    const accepted = await onSend(trimmed);
    if (accepted) {
      setText("");
      setRefused(false);
    } else {
      setRefused(true);
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      void submit();
    }
  };

  return (
    <div className="interaction-card interaction-card--inject">
      <div className="interaction-card__head">
        <span className="eyebrow">Clarification</span>
        <h3>Steer the running node</h3>
      </div>
      <p className="muted small">
        Queued for the node that is running now. It does not block the run.
      </p>
      <form className="form" onSubmit={submit}>
        <label className="field__label" htmlFor="inject-text">
          Message
        </label>
        <textarea
          id="inject-text"
          className="input"
          rows={3}
          value={text}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={onKeyDown}
          placeholder="Add a fact, a constraint, or a correction…"
          disabled={busy}
        />
        {refused ? (
          <p className="field__error" role="alert">
            The clarification was not accepted. See the message above.
          </p>
        ) : null}
        <div className="interaction-card__actions">
          <Button
            type="submit"
            variant="secondary"
            loading={busy}
            disabled={busy || !text.trim()}
          >
            Send
          </Button>
          <span className="muted small">Ctrl/Cmd + Enter</span>
        </div>
      </form>
    </div>
  );
}
