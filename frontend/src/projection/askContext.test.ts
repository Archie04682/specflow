import { describe, expect, it } from "vitest";
import type { StreamItem } from "./eventProjection";
import { recoverQuestionDescription } from "./askContext";

const at = "2026-01-01T12:00:00Z";

function askTool(id: number, nodeId: string, question: string): StreamItem {
  return {
    id,
    at,
    nodeId,
    kind: "tool",
    callId: `c-${id}`,
    name: "specflow_ask",
    args: { question, title: "Pick" },
    state: "running",
    files: [],
  };
}

function question(id: number, nodeId: string, questionId: string): StreamItem {
  return {
    id,
    at,
    nodeId,
    kind: "interaction",
    eventType: "question.asked",
    data: { id: questionId, kind: "select", title: "Pick" },
  };
}

describe("recoverQuestionDescription", () => {
  it("recovers the question from the ask tool call before it", () => {
    const items = [
      askTool(1, "explore", "Which extension point?"),
      question(2, "explore", "q1"),
    ];
    expect(recoverQuestionDescription(items, "explore", "q1")).toBe(
      "Which extension point?",
    );
  });

  it("uses the ask call for the newest question, not an older one", () => {
    const items = [
      askTool(1, "explore", "First question"),
      question(2, "explore", "q1"),
      askTool(3, "explore", "Second question"),
      question(4, "explore", "q2"),
    ];
    expect(recoverQuestionDescription(items, "explore", "q2")).toBe("Second question");
  });

  it("ignores ask calls on another node", () => {
    const items = [
      askTool(1, "propose", "Not this node"),
      question(2, "explore", "q1"),
    ];
    expect(recoverQuestionDescription(items, "explore", "q1")).toBeUndefined();
  });

  it("does not borrow from a question before another interaction", () => {
    const items = [
      askTool(1, "explore", "First question"),
      question(2, "explore", "q1"),
      { id: 3, at, nodeId: "explore", kind: "status" as const, text: "waiting" },
      question(4, "explore", "q2"),
    ];
    expect(recoverQuestionDescription(items, "explore", "q2")).toBeUndefined();
  });

  it("skips tool calls that carry no question", () => {
    const items = [
      askTool(1, "explore", "Which one?"),
      {
        id: 2,
        at,
        nodeId: "explore",
        kind: "tool" as const,
        callId: "c-bash",
        name: "bash",
        args: { command: "ls" },
        state: "ok" as const,
        files: [],
      },
      question(3, "explore", "q1"),
    ];
    expect(recoverQuestionDescription(items, "explore", "q1")).toBe("Which one?");
  });
});
