import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { StreamItem } from "../../../projection/eventProjection";
import { EventList } from "./EventList";

function statusItem(index: number): StreamItem {
  return {
    id: index,
    at: "2026-01-01T12:00:00Z",
    kind: "status",
    text: `item-${index}`,
  };
}

function makeScrollable(element: HTMLElement, height: number, clientHeight: number) {
  let scrollTop = 0;
  Object.defineProperty(element, "scrollHeight", {
    configurable: true,
    get: () => height,
  });
  Object.defineProperty(element, "clientHeight", {
    configurable: true,
    get: () => clientHeight,
  });
  Object.defineProperty(element, "scrollTop", {
    configurable: true,
    get: () => scrollTop,
    set: (value: number) => {
      scrollTop = value;
    },
  });
  return {
    get scrollTop() {
      return scrollTop;
    },
    set scrollTop(value: number) {
      scrollTop = value;
    },
  };
}

describe("EventList", () => {
  it("follows the newest event while pinned to the bottom", () => {
    const { container, rerender } = render(<EventList items={[statusItem(1)]} />);
    const list = container.querySelector(".event-list") as HTMLElement;
    const state = makeScrollable(list, 1000, 400);

    rerender(<EventList items={[statusItem(1), statusItem(2)]} />);
    expect(state.scrollTop).toBe(1000);
  });

  it("stops following when the operator scrolls up and resumes on demand", async () => {
    const user = userEvent.setup();
    const { container, rerender } = render(<EventList items={[statusItem(1)]} />);
    const list = container.querySelector(".event-list") as HTMLElement;
    const state = makeScrollable(list, 1000, 400);

    state.scrollTop = 100;
    fireEvent.scroll(list);
    rerender(<EventList items={[statusItem(1), statusItem(2)]} />);
    expect(state.scrollTop).toBe(100);

    await user.click(screen.getByRole("button", { name: "Jump to live" }));
    expect(state.scrollTop).toBe(1000);
  });

  it("renders a window and reveals earlier items on demand", async () => {
    const user = userEvent.setup();
    const items = Array.from({ length: 200 }, (_, index) => statusItem(index + 1));
    render(<EventList items={items} />);

    expect(screen.queryByText("item-1")).toBeNull();
    expect(screen.getByText("item-200")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show earlier (120)" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Show earlier (120)" }));
    expect(screen.getByText("item-1")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Show earlier/ })).toBeNull();
  });

  it("keeps an expanded thinking block open across updates and window changes", async () => {
    const user = userEvent.setup();
    const thinking: StreamItem = {
      id: 1,
      at: "2026-01-01T12:00:00Z",
      nodeId: "explore",
      kind: "thinking",
      text: "reasoning alpha",
      complete: true,
    };
    const filler = Array.from({ length: 85 }, (_, index) => statusItem(index + 2));
    const { rerender } = render(<EventList items={[thinking, ...filler]} />);

    await user.click(screen.getByRole("button", { name: "Show earlier (6)" }));
    await user.click(screen.getByRole("button", { name: /thinking/i }));
    expect(screen.getByText("reasoning alpha")).toBeInTheDocument();

    rerender(<EventList items={[thinking, ...filler, statusItem(200)]} />);
    expect(screen.getByText("reasoning alpha")).toBeInTheDocument();
  });
});
