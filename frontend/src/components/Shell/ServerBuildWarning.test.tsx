import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { ServerIdentity } from "../../types/server";
import { ServerBuildWarning } from "./ServerBuildWarning";

const stale: ServerIdentity = {
  revision: "abcdef123456",
  startedAt: "2026-11-15T10:00:00Z",
  checkout: { revision: "123456abcdef", staleReason: "checkout-ahead" },
};

describe("ServerBuildWarning", () => {
  it("names the running build, the checkout, and the restart action", () => {
    render(<ServerBuildWarning identity={stale} />);

    const banner = screen.getByRole("status");
    expect(banner).toHaveTextContent("abcdef12");
    expect(banner).toHaveTextContent("123456ab");
    expect(banner).toHaveTextContent(/Restart the SpecFlow server/);
  });

  it("explains newer sources rather than a moved checkout", () => {
    render(
      <ServerBuildWarning
        identity={{
          ...stale,
          checkout: { revision: "123456abcdef", staleReason: "sources-newer" },
        }}
      />,
    );

    const banner = screen.getByRole("status");
    expect(banner).toHaveTextContent(/predates backend source changes/);
    expect(banner).toHaveTextContent(/Restart the SpecFlow server/);
  });

  it("stays silent for a current build", () => {
    render(
      <ServerBuildWarning
        identity={{ ...stale, checkout: { revision: "123456abcdef" } }}
      />,
    );

    expect(screen.queryByRole("status")).toBeNull();
  });

  it("stays silent without an identity", () => {
    render(<ServerBuildWarning identity={null} />);

    expect(screen.queryByRole("status")).toBeNull();
  });

  it("falls back to a generic warning for an unrecognized reason", () => {
    render(
      <ServerBuildWarning
        identity={{
          ...stale,
          checkout: { revision: "123456abcdef", staleReason: "something-new" },
        }}
      />,
    );

    const banner = screen.getByRole("status");
    expect(banner).toHaveTextContent(/is stale/);
    expect(banner).toHaveTextContent(/Restart the SpecFlow server/);
  });

  it("stays dismissed for one server process but returns for another", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<ServerBuildWarning identity={stale} />);

    await user.click(screen.getByRole("button", { name: "Dismiss message" }));
    expect(screen.queryByRole("status")).toBeNull();

    // Same process: still dismissed.
    rerender(<ServerBuildWarning identity={stale} />);
    expect(screen.queryByRole("status")).toBeNull();

    // A different process: the warning returns.
    rerender(
      <ServerBuildWarning identity={{ ...stale, startedAt: "2026-11-15T12:00:00Z" }} />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("abcdef12");
  });
});
