import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RunActions } from "../../../hooks/useRunActions";
import { useRunStore } from "../../../stores/runStore";
import { makeRun } from "../../../test/fixtures";
import type { RunEvent } from "../../../types/events";
import { InteractionPanel } from "./InteractionPanel";

function makeActions(): RunActions {
  const accepted = async () => true;
  return {
    answer: vi.fn(accepted),
    inject: vi.fn(accepted),
    approve: vi.fn(accepted),
    cancel: vi.fn(accepted),
    retry: vi.fn(accepted),
    resume: vi.fn(accepted),
    apply: vi.fn(accepted),
    discard: vi.fn(accepted),
  };
}

function questionAsked(seq: number, id: string): RunEvent {
  return {
    seq,
    at: "2026-01-01T12:00:00Z",
    runId: "run-1",
    nodeId: "explore",
    type: "question.asked",
    data: {
      id,
      kind: "confirm",
      title: "Continue?",
      message: "Use OAuth?",
      attempt: 1,
    },
  };
}

describe("InteractionPanel", () => {
  beforeEach(() => {
    useRunStore.getState().resetRun();
  });

  it("waits for the question kind before enabling controls", () => {
    const run = makeRun({
      status: "waiting_answer",
      pending: {
        kind: "question",
        id: "q1",
        nodeId: "explore",
        title: "Continue?",
        requestedAt: "2026-01-01T12:00:00Z",
      },
    });

    render(<InteractionPanel run={run} actions={makeActions()} />);
    expect(
      screen.getByText(/Loading the question's answer options/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Yes" })).toBeNull();
  });

  it("enables the correct control once the question event arrives", () => {
    const run = makeRun({
      status: "waiting_answer",
      pending: {
        kind: "question",
        id: "q1",
        nodeId: "explore",
        requestedAt: "2026-01-01T12:00:00Z",
      },
    });

    render(<InteractionPanel run={run} actions={makeActions()} />);
    act(() => useRunStore.getState().ingestEvent(questionAsked(1, "q1")));

    expect(screen.getByRole("button", { name: "Yes" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "No" })).toBeInTheDocument();
  });

  it("decides an approval with the node and attempt", async () => {
    const user = userEvent.setup();
    const actions = makeActions();
    const run = makeRun({
      status: "waiting_approval",
      pending: {
        kind: "approval",
        id: "a1",
        nodeId: "merge",
        attempt: 2,
        title: "Merge",
        action: "git merge --no-ff specflow/run-run-1",
        requestedAt: "2026-01-01T12:00:00Z",
      },
    });

    render(<InteractionPanel run={run} actions={actions} />);
    await user.click(screen.getByRole("button", { name: "Approve" }));

    expect(actions.approve).toHaveBeenCalledWith("merge", 2, "approve");
  });

  it("recovers the description from the ask tool call when the event has none", () => {
    const run = makeRun({
      status: "waiting_answer",
      pending: {
        kind: "question",
        id: "q1",
        nodeId: "explore",
        title: "Plugin scope",
        requestedAt: "2026-01-01T12:00:00Z",
      },
    });

    render(<InteractionPanel run={run} actions={makeActions()} />);
    act(() => {
      useRunStore.getState().ingestEvent({
        seq: 1,
        at: "2026-01-01T12:00:00Z",
        runId: "run-1",
        nodeId: "explore",
        type: "node.event",
        data: {
          kind: "tool_started",
          tool: {
            callId: "c1",
            name: "specflow_ask",
            args: {
              question: "Which extension point matters?",
              title: "Plugin scope",
              options: ["Adapters", "Node kinds"],
            },
          },
        },
      });
      useRunStore.getState().ingestEvent({
        seq: 2,
        at: "2026-01-01T12:00:00Z",
        runId: "run-1",
        nodeId: "explore",
        type: "question.asked",
        data: {
          id: "q1",
          kind: "select",
          title: "Plugin scope",
          options: ["Adapters", "Node kinds"],
          attempt: 1,
        },
      });
    });

    expect(screen.getByText("Which extension point matters?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Adapters" })).toBeInTheDocument();
  });

  it("offers injection only while a node is running", () => {
    const { rerender } = render(
      <InteractionPanel run={makeRun({ status: "running" })} actions={makeActions()} />,
    );
    expect(screen.getByText("Steer the running node")).toBeInTheDocument();

    rerender(
      <InteractionPanel
        run={makeRun({ status: "completed", finishedAt: "2026-01-01T12:01:00Z" })}
        actions={makeActions()}
      />,
    );
    expect(screen.queryByText("Steer the running node")).toBeNull();
  });
});
