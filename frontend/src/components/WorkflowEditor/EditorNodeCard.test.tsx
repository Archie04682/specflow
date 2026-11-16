import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { EditorNodeCardView } from "./EditorNodeCard";

describe("EditorNodeCardView", () => {
  it("names the node and kind accessibly", () => {
    render(
      <EditorNodeCardView
        title="Explore"
        kind="agent"
        selected={false}
        onSelect={() => {}}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Explore, agent" }),
    ).toBeInTheDocument();
  });

  it("exposes its selected state", () => {
    render(
      <EditorNodeCardView
        title="Apply"
        kind="command"
        selected
        onSelect={() => {}}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Apply, command" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("selects on click and keyboard", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(
      <EditorNodeCardView
        title="Review"
        kind="agent"
        selected={false}
        onSelect={onSelect}
      />,
    );

    const button = screen.getByRole("button", { name: "Review, agent" });
    await user.click(button);
    expect(onSelect).toHaveBeenCalledTimes(1);

    button.focus();
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledTimes(2);
  });
});
