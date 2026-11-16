import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectPickerField } from "./ProjectPickerField";
import { callMethod, callUrl, fakeResponse, stubFetch } from "../../test/http";

function Harness({ initial = "" }: { initial?: string }) {
  const [value, setValue] = useState(initial);
  return <ProjectPickerField id="project" value={value} onChange={setValue} />;
}

describe("ProjectPickerField", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("fills the field with the chosen folder", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch(() => fakeResponse(200, { path: "/tmp/chosen" }));

    render(<Harness initial="/tmp/old" />);
    await user.click(screen.getByRole("button", { name: /Browse/ }));

    expect(screen.getByLabelText("Project")).toHaveValue("/tmp/chosen");
    expect(callMethod(fetchMock)).toBe("POST");
    expect(callUrl(fetchMock)).toBe("/api/project/pick");
  });

  it("leaves the field unchanged when the dialog is cancelled", async () => {
    const user = userEvent.setup();
    stubFetch(() => fakeResponse(200, { cancelled: true }));

    render(<Harness initial="/tmp/old" />);
    await user.click(screen.getByRole("button", { name: /Browse/ }));

    expect(screen.getByLabelText("Project")).toHaveValue("/tmp/old");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("keeps manual entry when the dialog is unavailable", async () => {
    const user = userEvent.setup();
    stubFetch(() =>
      fakeResponse(501, { error: "no native folder dialog is available" }),
    );

    render(<Harness initial="/tmp/old" />);
    await user.click(screen.getByRole("button", { name: /Browse/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /no native folder dialog is available/,
    );
    const input = screen.getByLabelText("Project");
    await user.clear(input);
    await user.type(input, "/tmp/typed");
    expect(input).toHaveValue("/tmp/typed");
  });

  it("reports a network failure and keeps the field usable", async () => {
    const user = userEvent.setup();
    stubFetch(() => {
      throw new TypeError("failed to fetch");
    });

    render(<Harness initial="/tmp/old" />);
    await user.click(screen.getByRole("button", { name: /Browse/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /Cannot reach the SpecFlow server/,
    );
    expect(screen.getByLabelText("Project")).toBeEnabled();
  });

  it("disables the button while the dialog is open", async () => {
    const user = userEvent.setup();
    let release: (() => void) | undefined;
    stubFetch(
      () =>
        new Promise((resolve) => {
          release = () => resolve(fakeResponse(200, { cancelled: true }));
        }),
    );

    render(<Harness initial="/tmp/old" />);
    const button = screen.getByRole("button", { name: /Browse/ });
    await user.click(button);
    expect(button).toBeDisabled();

    release?.();
    expect(await screen.findByRole("button", { name: /Browse/ })).toBeEnabled();
  });
});
