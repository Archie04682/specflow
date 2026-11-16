import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SettingsDialog } from "./SettingsDialog";

function renderSettings(
  overrides: Partial<Parameters<typeof SettingsDialog>[0]> = {},
) {
  const onAdd = vi.fn();
  const onSelect = vi.fn();
  const onRemove = vi.fn();
  const onClose = vi.fn();
  render(
    <SettingsDialog
      open
      projects={["/work/repo", "/personal/repo"]}
      activeProject="/work/repo"
      onAdd={onAdd}
      onSelect={onSelect}
      onRemove={onRemove}
      onClose={onClose}
      {...overrides}
    />,
  );
  return { onAdd, onSelect, onRemove, onClose };
}

describe("SettingsDialog", () => {
  it("lists projects with the active one marked and selects another", async () => {
    const user = userEvent.setup();
    const { onSelect } = renderSettings();

    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByRole("button", { name: "Use project repo (work)" }),
    ).toHaveAttribute("aria-current", "true");

    await user.click(
      within(dialog).getByRole("button", {
        name: "Use project repo (personal)",
      }),
    );
    expect(onSelect).toHaveBeenCalledWith("/personal/repo");
  });

  it("adds a project from a trimmed path", async () => {
    const user = userEvent.setup();
    const { onAdd } = renderSettings();
    const dialog = screen.getByRole("dialog");

    await user.type(
      within(dialog).getByLabelText("Project"),
      "  /another/repo  ",
    );
    await user.click(within(dialog).getByRole("button", { name: "Add project" }));
    expect(onAdd).toHaveBeenCalledWith("/another/repo");
  });

  it("removes a project from the workbench", async () => {
    const user = userEvent.setup();
    const { onRemove } = renderSettings();
    const dialog = screen.getByRole("dialog");

    await user.click(
      within(dialog).getByRole("button", {
        name: "Remove repo (personal) from the workbench",
      }),
    );
    expect(onRemove).toHaveBeenCalledWith("/personal/repo");
  });

  it("disables Add project for an empty path", () => {
    renderSettings();
    expect(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Add project",
      }),
    ).toBeDisabled();
  });

  it("shows the add form first when no projects exist", () => {
    renderSettings({ projects: [], activeProject: "" });
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/Add the first one below/)).toBeInTheDocument();
  });

  it("closes with Escape and Close", async () => {
    const user = userEvent.setup();
    const { onClose } = renderSettings();

    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("shows the running server build and an up-to-date checkout", () => {
    renderSettings({
      identity: {
        revision: "abcdef123456",
        builtAt: "2026-11-15T19:11:46Z",
        startedAt: "2026-11-15T19:12:00Z",
        checkout: { revision: "abcdef123456" },
      },
    });

    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Build")).toBeInTheDocument();
    expect(within(dialog).getByText("abcdef12")).toBeInTheDocument();
    expect(within(dialog).getByText("Built")).toBeInTheDocument();
    expect(within(dialog).getByText("Started")).toBeInTheDocument();
    expect(within(dialog).getByText(/up to date/)).toBeInTheDocument();
  });

  it("reports a checkout the running build lags", () => {
    renderSettings({
      identity: {
        revision: "abcdef123456",
        startedAt: "2026-11-15T19:12:00Z",
        checkout: { revision: "ffffffff999999", staleReason: "checkout-ahead" },
      },
    });

    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/restart needed/)).toBeInTheDocument();
    expect(within(dialog).getByText(/ffffffff/)).toBeInTheDocument();
  });

  it("says when the server is not running from source", () => {
    renderSettings({
      identity: { revision: "abcdef123456", startedAt: "2026-11-15T19:12:00Z" },
    });

    expect(
      within(screen.getByRole("dialog")).getByText(/not running from source/),
    ).toBeInTheDocument();
  });

  it("says when the server identity is unavailable", () => {
    renderSettings({ identity: null });

    expect(
      within(screen.getByRole("dialog")).getByText(
        /The server identity is unavailable/,
      ),
    ).toBeInTheDocument();
  });

  it("does not call the identity unavailable while the first read is in flight", () => {
    renderSettings({ identity: null, identityLoading: true });

    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/Reading the server identity/)).toBeInTheDocument();
    expect(
      within(dialog).queryByText(/The server identity is unavailable/),
    ).toBeNull();
  });

  it("labels a build without a revision as unknown", () => {
    renderSettings({ identity: { startedAt: "2026-11-15T19:12:00Z" } });

    expect(
      within(screen.getByRole("dialog")).getByText("unknown"),
    ).toBeInTheDocument();
  });
});
