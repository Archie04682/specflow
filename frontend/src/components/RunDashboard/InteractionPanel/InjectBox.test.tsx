import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InjectBox } from "./InjectBox";

describe("InjectBox", () => {
  it("queues a clarification and clears the field on success", async () => {
    const user = userEvent.setup();
    const onSend = vi.fn(async () => true);
    render(<InjectBox busy={false} onSend={onSend} />);

    const input = screen.getByLabelText("Message");
    await user.type(input, "use encoding/csv");
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(onSend).toHaveBeenCalledWith("use encoding/csv");
    expect(input).toHaveValue("");
  });

  it("keeps the text and explains a refusal", async () => {
    const user = userEvent.setup();
    const onSend = vi.fn(async () => false);
    render(<InjectBox busy={false} onSend={onSend} />);

    const input = screen.getByLabelText("Message");
    await user.type(input, "keep this");
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(input).toHaveValue("keep this");
    expect(screen.getByRole("alert")).toHaveTextContent(/not accepted/);
  });

  it("disables send for an empty message", () => {
    render(<InjectBox busy={false} onSend={async () => true} />);
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  });
});
