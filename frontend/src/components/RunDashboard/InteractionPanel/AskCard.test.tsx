import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { AskShape } from "../../../types/events";
import type { PendingInteraction } from "../../../types/run";
import { AskCard } from "./AskCard";

const question: PendingInteraction = {
  kind: "question",
  id: "q1",
  nodeId: "explore",
  attempt: 1,
  title: "Pick an approach",
  message: "How should this be implemented?",
  requestedAt: "2026-01-01T12:00:00Z",
  deadline: "2026-01-01T12:30:00Z",
};

function setup(ask: AskShape) {
  const onSubmit = vi.fn(async () => true);
  render(<AskCard question={question} ask={ask} busy={false} onSubmit={onSubmit} />);
  return onSubmit;
}

describe("AskCard", () => {
  it("sends the confirmed flag for a confirmation question", async () => {
    const user = userEvent.setup();
    const onSubmit = setup({ kind: "confirm", title: "Continue?", message: "Use OAuth?" });

    await user.click(screen.getByRole("button", { name: "Yes" }));
    expect(onSubmit).toHaveBeenLastCalledWith({ questionId: "q1", confirmed: true });

    await user.click(screen.getByRole("button", { name: "No" }));
    expect(onSubmit).toHaveBeenLastCalledWith({ questionId: "q1", confirmed: false });
  });

  it("sends the chosen option as the value", async () => {
    const user = userEvent.setup();
    const onSubmit = setup({ kind: "select", options: ["OAuth", "API tokens"] });

    await user.click(screen.getByRole("button", { name: "API tokens" }));
    expect(onSubmit).toHaveBeenCalledWith({ questionId: "q1", value: "API tokens" });
  });

  it("sends free text as the value", async () => {
    const user = userEvent.setup();
    const onSubmit = setup({ kind: "input", placeholder: "Type here" });

    await user.type(screen.getByLabelText("Answer"), "use encoding/csv");
    await user.click(screen.getByRole("button", { name: "Send answer" }));
    expect(onSubmit).toHaveBeenCalledWith({
      questionId: "q1",
      value: "use encoding/csv",
    });
  });

  it("starts an editor with its prefill", async () => {
    const user = userEvent.setup();
    const onSubmit = setup({ kind: "editor", prefill: "draft answer" });

    const editor = screen.getByLabelText("Answer");
    expect(editor).toHaveValue("draft answer");

    await user.clear(editor);
    await user.type(editor, "final answer");
    await user.click(screen.getByRole("button", { name: "Send answer" }));
    expect(onSubmit).toHaveBeenCalledWith({ questionId: "q1", value: "final answer" });
  });

  it("cancels the question", async () => {
    const user = userEvent.setup();
    const onSubmit = setup({ kind: "input" });

    await user.click(screen.getByRole("button", { name: "Cancel question" }));
    expect(onSubmit).toHaveBeenCalledWith({ questionId: "q1", cancelled: true });
  });

  it("shows the question's description as markdown above the controls", () => {
    const { container } = render(
      <AskCard
        question={question}
        ask={{
          kind: "select",
          title: "Pick an approach",
          message: "Consider **both** pages before choosing.",
          options: ["OAuth", "API tokens"],
        }}
        busy={false}
        onSubmit={vi.fn(async () => true)}
      />,
    );

    expect(screen.getByText("both").tagName).toBe("STRONG");
    const description = container.querySelector(".interaction-card__message");
    const firstOption = screen.getByRole("button", { name: "OAuth" });
    expect(description).not.toBeNull();
    expect(
      description!.compareDocumentPosition(firstOption) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("shows no deadline for an unbounded question", () => {
    render(
      <AskCard
        question={{ ...question, deadline: undefined }}
        ask={{ kind: "select", title: "Pick one", options: ["A", "B"] }}
        busy={false}
        onSubmit={vi.fn(async () => true)}
      />,
    );
    expect(screen.getByText(/no deadline/)).toBeInTheDocument();
  });

  it("renders a question without a description", () => {
    render(
      <AskCard
        question={{ ...question, message: undefined }}
        ask={{ kind: "select", title: "Pick one", options: ["A", "B"] }}
        busy={false}
        onSubmit={vi.fn(async () => true)}
      />,
    );
    expect(screen.getByText("Pick one")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "A" })).toBeInTheDocument();
  });
});
