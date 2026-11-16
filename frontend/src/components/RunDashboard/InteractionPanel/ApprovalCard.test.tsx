import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { PendingInteraction } from "../../../types/run";
import { ApprovalCard } from "./ApprovalCard";

const pending: PendingInteraction = {
  kind: "approval",
  id: "a1",
  nodeId: "merge",
  attempt: 2,
  title: "Merge the run branch",
  action: "git merge --no-ff specflow/run-run-1",
  requestedAt: "2026-01-01T12:00:00Z",
  deadline: "2026-01-01T12:30:00Z",
};

describe("ApprovalCard", () => {
  it("shows the action, attempt, and the consequence of rejecting", () => {
    render(
      <ApprovalCard pending={pending} busy={false} onDecide={async () => true} />,
    );

    expect(screen.getByText("Merge the run branch")).toBeInTheDocument();
    expect(screen.getByText(/attempt 2/)).toBeInTheDocument();
    expect(screen.getByText(/git merge --no-ff/)).toBeInTheDocument();
    expect(screen.getByText(/Rejecting ends the run cancelled/)).toBeInTheDocument();
  });

  it("sends each decision", async () => {
    const user = userEvent.setup();
    const onDecide = vi.fn(async () => true);
    render(<ApprovalCard pending={pending} busy={false} onDecide={onDecide} />);

    await user.click(screen.getByRole("button", { name: "Approve" }));
    expect(onDecide).toHaveBeenLastCalledWith("approve");

    await user.click(screen.getByRole("button", { name: "Reject" }));
    expect(onDecide).toHaveBeenLastCalledWith("reject");
  });

  it("disables both decisions while one is in flight", () => {
    render(<ApprovalCard pending={pending} busy onDecide={async () => true} />);

    expect(screen.getByRole("button", { name: "Reject" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Approve" })).toBeDisabled();
  });

  it("shows a configured deadline and omits an unbounded one", () => {
    const { rerender } = render(
      <ApprovalCard pending={pending} busy={false} onDecide={async () => true} />,
    );
    expect(screen.getByText(/deadline/)).toBeInTheDocument();

    rerender(
      <ApprovalCard
        pending={{ ...pending, deadline: undefined }}
        busy={false}
        onDecide={async () => true}
      />,
    );
    expect(screen.queryByText(/deadline/)).toBeNull();
  });
});
