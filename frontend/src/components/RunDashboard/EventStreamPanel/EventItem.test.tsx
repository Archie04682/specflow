import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { StreamItem } from "../../../projection/eventProjection";
import { EventItem } from "./EventItem";

const at = "2026-01-01T12:00:00Z";

function renderItem(item: StreamItem) {
  return render(<EventItem item={item} />);
}

describe("EventItem", () => {
  it("renders a lifecycle banner", () => {
    renderItem({
      id: 1,
      at,
      kind: "lifecycle",
      eventType: "run.completed",
      data: { summary: "3/3 nodes completed in 2m" },
    });
    expect(screen.getByText("3/3 nodes completed in 2m")).toBeInTheDocument();
  });

  it("renders a node failure with its error", () => {
    renderItem({
      id: 2,
      at,
      nodeId: "apply",
      kind: "node",
      eventType: "node.failed",
      data: { error: "postcondition failed" },
    });
    expect(screen.getByText(/Apply failed|apply failed/i)).toBeInTheDocument();
    expect(screen.getByText("postcondition failed")).toBeInTheDocument();
  });

  it("renders a question card with its options", () => {
    renderItem({
      id: 3,
      at,
      nodeId: "propose",
      kind: "interaction",
      eventType: "question.asked",
      data: {
        id: "q1",
        title: "Choose auth",
        message: "Use OAuth?",
        options: ["OAuth", "Tokens"],
      },
    });
    expect(screen.getByText("Choose auth")).toBeInTheDocument();
    expect(screen.getByText("Use OAuth?")).toBeInTheDocument();
    expect(screen.getByText("OAuth")).toBeInTheDocument();
    expect(screen.getByText(/Answer in the interaction panel/)).toBeInTheDocument();
  });

  it("collapses thinking blocks by default", () => {
    renderItem({
      id: 4,
      at,
      nodeId: "explore",
      kind: "thinking",
      text: "considering the options",
      complete: true,
    });
    const toggle = screen.getByRole("button", { name: /thinking/i });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("considering the options")).not.toBeInTheDocument();
    expect(screen.getByText("23 chars")).toBeInTheDocument();
  });

  it("expands a thinking block into markdown", () => {
    render(
      <EventItem
        item={{
          id: 5,
          at,
          nodeId: "explore",
          kind: "thinking",
          text: "**bold reason**",
          complete: true,
        }}
        expanded
        onToggleThinking={() => {}}
      />,
    );
    expect(screen.getByRole("button", { name: /thinking/i })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByText("bold reason").tagName).toBe("STRONG");
  });

  it("reports a streaming thinking block without showing its text", () => {
    renderItem({
      id: 6,
      at,
      nodeId: "explore",
      kind: "thinking",
      text: "considering the options",
      complete: false,
    });
    expect(screen.getByText("streaming")).toBeInTheDocument();
    expect(screen.queryByText("considering the options")).not.toBeInTheDocument();
  });

  it("renders assistant text as markdown once complete", () => {
    renderItem({
      id: 7,
      at,
      nodeId: "explore",
      kind: "message",
      text: "## Result\n\n- one\n- two",
      complete: true,
    });
    expect(screen.getByRole("heading", { name: "Result" })).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByText("Assistant")).toBeInTheDocument();
  });

  it("renders assistant text plainly while streaming", () => {
    renderItem({
      id: 8,
      at,
      nodeId: "explore",
      kind: "message",
      text: "**still** typing",
      complete: false,
    });
    expect(screen.getByText("**still** typing")).toBeInTheDocument();
  });

  it("renders one tool card with arguments, summary, and files", () => {
    renderItem({
      id: 6,
      at,
      nodeId: "explore",
      kind: "tool",
      callId: "c1",
      name: "read",
      args: { path: "main.go" },
      summary: "read 3 lines",
      state: "ok",
      durationMs: 12,
      files: ["main.go"],
    });

    expect(screen.getByText("read")).toBeInTheDocument();
    expect(screen.getByText("read 3 lines")).toBeInTheDocument();
    expect(screen.getByText("12ms")).toBeInTheDocument();
    expect(screen.getAllByText(/main\.go/).length).toBeGreaterThan(0);
  });

  it("renders file, notice, status, and unknown rows", () => {
    const { rerender } = renderItem({
      id: 7,
      at,
      nodeId: "apply",
      kind: "file",
      path: "web/app.tsx",
      tool: "write",
    });
    expect(screen.getByText("web/app.tsx")).toBeInTheDocument();

    rerender(
      <EventItem
        item={{
          id: 8,
          at,
          kind: "notice",
          level: "warning",
          message: "context compacted",
        }}
      />,
    );
    expect(screen.getByText("context compacted")).toBeInTheDocument();

    rerender(
      <EventItem item={{ id: 9, at, kind: "status", text: "waiting on tests" }} />,
    );
    expect(screen.getByText("waiting on tests")).toBeInTheDocument();

    rerender(
      <EventItem item={{ id: 10, at, kind: "unknown", kindName: "mystery_event" }} />,
    );
    expect(screen.getByText(/Unrecognized event kind/)).toBeInTheDocument();
    expect(screen.getByText("mystery_event")).toBeInTheDocument();
  });

  it("renders a command tool as a shell line with terminal output", () => {
    renderItem({
      id: 11,
      at,
      nodeId: "apply",
      kind: "tool",
      callId: "c2",
      name: "bash",
      args: { command: "make test", timeout: 30 },
      summary: "ok\n2 passed",
      state: "ok",
      durationMs: 1200,
      files: [],
    });

    expect(screen.getByText("make test")).toBeInTheDocument();
    expect(screen.getByText("1.2s")).toBeInTheDocument();
    expect(screen.getByText(/2 passed/)).toBeInTheDocument();
    expect(screen.queryByText(/\{"command"/)).not.toBeInTheDocument();
  });

  it("renders a command node card with its exit code and output", () => {
    renderItem({
      id: 12,
      at,
      nodeId: "verify",
      kind: "command",
      command: "openspec validate activity-console --strict",
      state: "error",
      exitCode: 1,
      output: "change invalid",
      durationMs: 2400,
    });

    expect(
      screen.getByText(/openspec validate activity-console --strict/),
    ).toBeInTheDocument();
    expect(screen.getByText("exit 1")).toBeInTheDocument();
    expect(screen.getByText("change invalid")).toBeInTheDocument();
    expect(screen.getByText("2.4s")).toBeInTheDocument();
  });
});
