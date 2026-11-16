import type { RunActions } from "../../../hooks/useRunActions";
import { recoverQuestionDescription } from "../../../projection/askContext";
import { useAskById, usePendingAction, useStreamItems } from "../../../stores/runStore";
import type { Run } from "../../../types/run";
import { Spinner } from "../../common/Spinner";
import { ApprovalCard } from "./ApprovalCard";
import { AskCard } from "./AskCard";
import { InjectBox } from "./InjectBox";

interface InteractionPanelProps {
  run: Run;
  actions: RunActions;
}

export function InteractionPanel({ run, actions }: InteractionPanelProps) {
  const askById = useAskById();
  const pendingAction = usePendingAction();
  const items = useStreamItems();
  const pending = run.pending;

  if (pending?.kind === "question") {
    const ask = askById[pending.id];
    // The question event usually carries the description. When it does not
    // (a journal written before the adapter forwarded it), the ask tool call in
    // the same stream still holds the text the model asked with.
    const presentedAsk =
      ask && !ask.message
        ? {
            ...ask,
            message:
              pending.message ??
              recoverQuestionDescription(items, pending.nodeId, pending.id),
          }
        : ask;
    return (
      <section
        className="interaction-dock"
        aria-label="Interaction needed"
        aria-live="polite"
      >
        {ask ? (
          <AskCard
            question={pending}
            ask={presentedAsk}
            busy={pendingAction === "answer"}
            onSubmit={actions.answer}
          />
        ) : (
          <div className="interaction-card" aria-busy="true">
            <div className="interaction-card__head">
              <span className="eyebrow">Question</span>
              <h3>{pending.title ?? "The run needs an answer"}</h3>
            </div>
            <p className="muted small">
              Loading the question's answer options from the event stream…
            </p>
            <Spinner size="sm" />
          </div>
        )}
      </section>
    );
  }

  if (pending?.kind === "approval") {
    return (
      <section
        className="interaction-dock"
        aria-label="Interaction needed"
        aria-live="polite"
      >
        <ApprovalCard
          pending={pending}
          busy={pendingAction === "approve"}
          onDecide={(decision) =>
            actions.approve(pending.nodeId, pending.attempt ?? 0, decision)
          }
        />
      </section>
    );
  }

  if (run.status === "running") {
    return (
      <section className="interaction-dock" aria-label="Send a clarification">
        <InjectBox busy={pendingAction === "inject"} onSend={actions.inject} />
      </section>
    );
  }

  return null;
}
