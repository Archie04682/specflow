import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { StreamItem } from "../../../projection/eventProjection";
import { ToolCard } from "./ToolCard";

type ToolItem = Extract<StreamItem, { kind: "tool" }>;

const at = "2026-01-01T12:00:00Z";

function tool(overrides: Partial<ToolItem>): ToolItem {
  return {
    id: 1,
    at,
    nodeId: "explore",
    kind: "tool",
    callId: "c1",
    name: "bash",
    state: "ok",
    files: [],
    ...overrides,
  };
}

describe("ToolCard", () => {
  it("renders a shell command as a prompt line", () => {
    render(
      <ToolCard
        item={tool({ args: { command: "go test ./...", timeout: 60 }, summary: "ok" })}
      />,
    );
    expect(screen.getByText("go test ./...")).toBeInTheDocument();
    expect(screen.getByText("timeout 60s")).toBeInTheDocument();
    expect(screen.queryByText(/\{"command"/)).not.toBeInTheDocument();
  });

  it("marks a failed call as failed", () => {
    const { container } = render(
      <ToolCard item={tool({ state: "error", summary: "exit status 1" })} />,
    );
    expect(screen.getByText("failed")).toBeInTheDocument();
    expect(container.querySelector(".tool-card--error")).not.toBeNull();
    expect(container.querySelector(".terminal--failed")).not.toBeNull();
  });

  it("bounds long output and reveals it on request", () => {
    const output = Array.from({ length: 20 }, (_, index) => `line ${index}`).join("\n");
    render(<ToolCard item={tool({ summary: output })} />);
    const toggle = screen.getByRole("button", { name: "Show all (20 lines)" });
    expect(screen.getByText(/line 9/)).toBeInTheDocument();
    expect(screen.queryByText(/line 19/)).toBeNull();

    fireEvent.click(toggle);
    expect(screen.getByText(/line 19/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show less" })).toBeInTheDocument();
  });

  it("lists an unknown tool's arguments as labelled rows", () => {
    render(<ToolCard item={tool({ name: "mystery", args: { alpha: "one", beta: 2 } })} />);
    expect(screen.getByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("one")).toBeInTheDocument();
    expect(screen.getByText("beta")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("shows the ask tool as a question with its description and options", () => {
    render(
      <ToolCard
        item={tool({
          name: "specflow_ask",
          args: {
            title: "Pick a scope",
            question: "Which **extension point** matters?",
            options: ["Adapters", "Node kinds"],
          },
        })}
      />,
    );
    expect(screen.getByText("Pick a scope")).toBeInTheDocument();
    expect(screen.getByText("extension point").tagName).toBe("STRONG");
    expect(screen.getByText("Adapters")).toBeInTheDocument();
    expect(screen.getByText("Node kinds")).toBeInTheDocument();
  });
});
