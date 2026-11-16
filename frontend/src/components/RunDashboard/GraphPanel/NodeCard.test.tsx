import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { VisualState } from "../../../graph/nodeState";
import { NODE_STATE_LABEL } from "../../../graph/nodeState";
import { NodeCardView } from "./NodeCard";

describe("NodeCardView", () => {
  const states: VisualState[] = [
    "pending",
    "running",
    "waiting_approval",
    "waiting_answer",
    "completed",
    "failed",
    "cancelled",
    "skipped",
    "not_run",
  ];

  it.each(states)("states %s in the accessible name", (state) => {
    render(
      <NodeCardView title="Explore" kind="agent" state={state} onSelect={() => {}} />,
    );

    expect(
      screen.getByRole("button", {
        name: `Explore, ${NODE_STATE_LABEL[state]}`,
      }),
    ).toBeInTheDocument();
  });

  it("calls attention to a gate and its outcome", () => {
    const { rerender } = render(
      <NodeCardView
        title="Propose"
        kind="agent"
        state="waiting_approval"
        onSelect={() => {}}
      />,
    );
    expect(screen.getByText("Needs approval")).toBeInTheDocument();

    rerender(
      <NodeCardView title="Propose" kind="agent" state="failed" onSelect={() => {}} />,
    );
    expect(screen.getByText("View error")).toBeInTheDocument();
  });

  it("selects the node when activated", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(
      <NodeCardView title="Apply" kind="agent" state="running" onSelect={onSelect} />,
    );

    await user.click(screen.getByRole("button", { name: /Apply, Running/ }));
    expect(onSelect).toHaveBeenCalledTimes(1);
  });

  it("is operable from the keyboard", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(
      <NodeCardView title="Apply" kind="agent" state="running" onSelect={onSelect} />,
    );

    await user.tab();
    expect(screen.getByRole("button", { name: /Apply, Running/ })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledTimes(1);
  });
});
