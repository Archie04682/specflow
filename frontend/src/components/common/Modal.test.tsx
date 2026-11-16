import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./Modal";

describe("ConfirmDialog", () => {
  it("focuses the confirm action and cancels on Escape", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    const onConfirm = vi.fn();

    render(
      <ConfirmDialog
        open
        title="Cancel this run?"
        confirmLabel="Cancel run"
        onConfirm={onConfirm}
        onCancel={onCancel}
      >
        Body copy
      </ConfirmDialog>,
    );

    expect(screen.getByRole("button", { name: "Cancel run" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("traps Tab between its actions", async () => {
    const user = userEvent.setup();
    render(
      <ConfirmDialog
        open
        title="Cancel this run?"
        confirmLabel="Cancel run"
        onConfirm={() => {}}
        onCancel={() => {}}
      >
        Body copy
      </ConfirmDialog>,
    );

    const cancel = screen.getByRole("button", { name: "Cancel" });
    await user.tab();
    expect(cancel).toHaveFocus();
  });

  it("runs the confirm action", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        open
        title="Cancel this run?"
        confirmLabel="Cancel run"
        onConfirm={onConfirm}
        onCancel={() => {}}
      >
        Body copy
      </ConfirmDialog>,
    );

    await user.click(screen.getByRole("button", { name: "Cancel run" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("renders nothing when closed", () => {
    render(
      <ConfirmDialog
        open={false}
        title="Hidden"
        confirmLabel="Confirm"
        onConfirm={() => {}}
        onCancel={() => {}}
      >
        Body copy
      </ConfirmDialog>,
    );
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
});
