import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DEFAULT_RUN_SIDE, MIN_RUN_SIDE } from "../../lib/split";
import { RunSplit } from "./RunSplit";

function renderSplit() {
  return render(
    <RunSplit graph={<div>graph</div>} side={<div>activity</div>} busy />,
  );
}

describe("RunSplit", () => {
  it("renders both panes and an adjustable separator", () => {
    renderSplit();
    expect(screen.getByText("graph")).toBeInTheDocument();
    expect(screen.getByText("activity")).toBeInTheDocument();
    const handle = screen.getByRole("separator", { name: "Resize activity panel" });
    expect(handle).toHaveAttribute("aria-valuenow", String(DEFAULT_RUN_SIDE));
    expect(handle).toHaveAttribute("tabindex", "0");
  });

  it("adjusts the width from the keyboard and remembers it", () => {
    renderSplit();
    const handle = screen.getByRole("separator", { name: "Resize activity panel" });
    fireEvent.keyDown(handle, { key: "ArrowLeft" });
    expect(handle).toHaveAttribute("aria-valuenow", String(DEFAULT_RUN_SIDE + 24));
    expect(localStorage.getItem("specflow.runSplit")).toBe(String(DEFAULT_RUN_SIDE + 24));
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(handle).toHaveAttribute("aria-valuenow", String(DEFAULT_RUN_SIDE));
  });

  it("moves to the bounds with home and end", () => {
    renderSplit();
    const handle = screen.getByRole("separator", { name: "Resize activity panel" });
    fireEvent.keyDown(handle, { key: "Home" });
    expect(handle).toHaveAttribute("aria-valuenow", String(MIN_RUN_SIDE));
    fireEvent.keyDown(handle, { key: "End" });
    expect(Number(handle.getAttribute("aria-valuenow"))).toBeGreaterThan(DEFAULT_RUN_SIDE);
  });

  it("restores the default on reset", () => {
    renderSplit();
    const handle = screen.getByRole("separator", { name: "Resize activity panel" });
    fireEvent.keyDown(handle, { key: "Home" });
    fireEvent.keyDown(handle, { key: "Enter" });
    expect(handle).toHaveAttribute("aria-valuenow", String(DEFAULT_RUN_SIDE));
    fireEvent.keyDown(handle, { key: "Home" });
    fireEvent.doubleClick(handle);
    expect(handle).toHaveAttribute("aria-valuenow", String(DEFAULT_RUN_SIDE));
  });

  it("restores a remembered width on mount", () => {
    localStorage.setItem("specflow.runSplit", "640");
    renderSplit();
    expect(
      screen.getByRole("separator", { name: "Resize activity panel" }),
    ).toHaveAttribute("aria-valuenow", "640");
  });

  it("widens the activity column when the separator is dragged left", () => {
    renderSplit();
    const handle = screen.getByRole("separator", { name: "Resize activity panel" });
    fireEvent.pointerDown(handle, { button: 0, clientX: 800, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientX: 750, pointerId: 1 });
    fireEvent.pointerUp(handle, { clientX: 750, pointerId: 1 });
    expect(handle).toHaveAttribute("aria-valuenow", String(DEFAULT_RUN_SIDE + 50));
  });
});
