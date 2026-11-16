import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { graphNode, makeNode, makeRun } from "../../../test/fixtures";
import { NodeDetails } from "./NodeDetails";

describe("NodeDetails", () => {
  it("shows the failure, checkpoint, and both retry modes", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    const onClose = vi.fn();
    const node = makeNode({
      nodeId: "apply",
      title: "Apply",
      status: "failed",
      attempts: 2,
      error: "postcondition failed: every task must be checked",
      failureClass: "postcondition",
      checkpoint: "abcdef1234567890",
      usage: { totalTokens: 1200, costUsd: 0.02 },
    });

    render(
      <NodeDetails
        node={node}
        graphNode={graphNode("apply")}
        run={makeRun({ status: "failed" })}
        busy={false}
        onClose={onClose}
        onRetry={onRetry}
      />,
    );

    expect(screen.getByText(/postcondition failed/)).toBeInTheDocument();
    expect(screen.getByText("postcondition")).toBeInTheDocument();
    expect(screen.getByText("abcdef123456")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Close node details" })).toHaveFocus();

    await user.click(screen.getByRole("button", { name: "Retry clean" }));
    expect(onRetry).toHaveBeenCalledWith("clean");

    await user.click(screen.getByRole("button", { name: "Retry forward" }));
    expect(onRetry).toHaveBeenCalledWith("forward");

    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not offer a retry for a completed node", () => {
    render(
      <NodeDetails
        node={makeNode({ nodeId: "explore", status: "completed" })}
        graphNode={graphNode("explore")}
        run={makeRun({ status: "running" })}
        busy={false}
        onClose={() => {}}
        onRetry={() => {}}
      />,
    );

    expect(screen.queryByRole("button", { name: "Retry forward" })).toBeNull();
  });

  it("shows a command node's command, exit code, and output", () => {
    render(
      <NodeDetails
        node={makeNode({
          nodeId: "verify",
          title: "Verify",
          kind: "command",
          status: "failed",
          exitCode: 1,
          command: "make test",
          output: "2 tests failed",
        })}
        graphNode={graphNode("verify", { kind: "command" })}
        run={makeRun({ status: "failed" })}
        busy={false}
        onClose={() => {}}
        onRetry={() => {}}
      />,
    );

    expect(screen.getByText("make test")).toBeInTheDocument();
    expect(screen.getByText("exit 1")).toBeInTheDocument();
    expect(screen.getByText(/2 tests failed/)).toBeInTheDocument();
  });

  it("omits the command section for a command node that has not run", () => {
    render(
      <NodeDetails
        node={makeNode({ nodeId: "gate", title: "Gate", kind: "command", status: "pending" })}
        graphNode={graphNode("gate", { kind: "command" })}
        run={makeRun({ status: "running" })}
        busy={false}
        onClose={() => {}}
        onRetry={() => {}}
      />,
    );

    expect(screen.queryByRole("heading", { name: "Command" })).toBeNull();
  });

  it("reads a completed command node as exit 0 when the record omits it", () => {
    render(
      <NodeDetails
        node={makeNode({
          nodeId: "gate",
          title: "Gate",
          kind: "command",
          status: "completed",
          command: "echo ok",
          output: "ok",
        })}
        graphNode={graphNode("gate", { kind: "command" })}
        run={makeRun({ status: "completed" })}
        busy={false}
        onClose={() => {}}
        onRetry={() => {}}
      />,
    );

    expect(screen.getByText("exit 0")).toBeInTheDocument();
    expect(screen.getByText("echo ok")).toBeInTheDocument();
  });
});
