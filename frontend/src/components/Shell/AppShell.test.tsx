import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";

function renderShell(overrides: Partial<Parameters<typeof AppShell>[0]> = {}) {
  const onCloseDrawer = vi.fn();
  const props = {
    panel: (
      <div>
        <button type="button">New chat</button>
        <button type="button">New flow</button>
      </div>
    ),
    collapsed: false,
    panelHidden: false,
    drawerOpen: false,
    onCloseDrawer,
    ...overrides,
  };
  const view = render(
    <AppShell {...props}>
      <h1>Workspace</h1>
    </AppShell>,
  );
  return { ...view, onCloseDrawer, props };
}

describe("AppShell", () => {
  it("exposes a skip link and the workspace landmark", () => {
    renderShell();

    expect(screen.getByRole("link", { name: "Skip to content" })).toHaveAttribute(
      "href",
      "#workspace",
    );
    expect(screen.getByRole("main")).toHaveAttribute("id", "workspace");
    expect(screen.getByRole("complementary")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Close control panel" }),
    ).not.toBeInTheDocument();
  });

  it("hides the panel from assistive technology when it is collapsed", () => {
    const { container } = renderShell({ collapsed: true, panelHidden: true });

    expect(container.querySelector(".shell")).toHaveAttribute(
      "data-panel",
      "closed",
    );
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
  });

  it("closes the drawer on Escape and returns focus to the trigger", async () => {
    const user = userEvent.setup();
    const trigger = document.createElement("button");
    trigger.textContent = "Open panel";
    document.body.append(trigger);
    trigger.focus();

    const { rerender, onCloseDrawer, props } = renderShell({ drawerOpen: false });
    rerender(
      <AppShell {...props} drawerOpen>
        <h1>Workspace</h1>
      </AppShell>,
    );

    const scrim = screen.getByRole("button", { name: "Close control panel" });
    expect(scrim).toBeInTheDocument();
    // The drawer moves focus into its first control.
    expect(screen.getByRole("button", { name: "New chat" })).toHaveFocus();

    await user.keyboard("{Escape}");
    expect(onCloseDrawer).toHaveBeenCalledTimes(1);

    // Closing unmounts the drawer state; the effect restores the trigger.
    rerender(
      <AppShell {...props} drawerOpen={false}>
        <h1>Workspace</h1>
      </AppShell>,
    );
    expect(trigger).toHaveFocus();
    trigger.remove();
  });
});
