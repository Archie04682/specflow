import type { StreamItem } from "./eventProjection";

/**
 * Recovers a pending question's description from the ask tool call that
 * produced it.
 *
 * Adapters that report the ask's message need nothing. This covers streams
 * whose question event carries no description anyway: a journal written before
 * the adapter learned to forward it, or a harness that keeps the text only in
 * the tool arguments. The most recent ask tool call before the question is its
 * context; an interaction or node boundary ends the search so an older ask can
 * never lend its text to a newer question.
 */
export function recoverQuestionDescription(
  items: StreamItem[],
  nodeId: string | undefined,
  questionId: string,
): string | undefined {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index]!;
    if (item.nodeId !== nodeId) continue;

    if (item.kind === "tool") {
      const question = item.args?.question;
      if (typeof question === "string" && question.length > 0) return question;
      continue;
    }

    if (item.kind === "interaction") {
      if (item.eventType === "question.asked" && item.data.id === questionId) {
        // The question's own event; its context lies before it.
        continue;
      }
      return undefined;
    }

    if (item.kind === "command" || item.kind === "node") return undefined;
  }
  return undefined;
}
