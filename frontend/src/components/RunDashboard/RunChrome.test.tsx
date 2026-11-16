import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RunControls } from "./RunControls";
import { RunFooter } from "./RunFooter";
import { makeNode, makeRun } from "../../test/fixtures";
import { formatClock } from "../../lib/format";
import type { Run } from "../../types/run";
import { useRunStore } from "../../stores/runStore";

describe("RunControls", () => {
  it("shows the controls a running run permits", async () => {
    const user = userEvent.setup();
    const onRequestCancel = vi.fn();

    render(
      <RunControls
        runStatus="running"
        pendingAction={null}
        onRequestCancel={onRequestCancel}
        onResume={() => {}}
        onRetry={() => {}}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Cancel run" }));
    expect(onRequestCancel).toHaveBeenCalledTimes(1);
  });

  it("offers retry and resume for a failed run", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    const onResume = vi.fn();

    render(
      <RunControls
        runStatus="failed"
        failedNode={{ nodeId: "apply", title: "Apply" }}
        pendingAction={null}
        onRequestCancel={() => {}}
        onResume={onResume}
        onRetry={onRetry}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Retry Apply" }));
    expect(onRetry).toHaveBeenCalledWith("apply");

    await user.click(screen.getByRole("button", { name: "Resume run" }));
    expect(onResume).toHaveBeenCalledTimes(1);
  });

  it("disables the controls while an action is in flight", () => {
    render(
      <RunControls
        runStatus="running"
        pendingAction="cancel"
        onRequestCancel={() => {}}
        onResume={() => {}}
        onRetry={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: "Cancel run" })).toBeDisabled();
  });
});

describe("RunFooter", () => {
  beforeEach(() => {
    useRunStore.getState().resetRun();
  });

  it("shows duration, node progress, usage, and the connection", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T12:01:30Z"));

    const run: Run = makeRun({
      startedAt: "2026-01-01T12:00:00Z",
      nodes: [
        makeNode({ nodeId: "a", status: "completed", usage: { totalTokens: 1200 } }),
        makeNode({ nodeId: "b", status: "running", usage: { totalTokens: 300, costUsd: 0.0123 } }),
      ],
    });
    const lastEventAt = "2026-01-01T12:01:20Z";
    useRunStore.getState().ingestEvent({
      seq: 1,
      at: lastEventAt,
      runId: run.id,
      nodeId: "a",
      type: "node.event",
      data: { kind: "status", text: "working" },
    });

    render(<RunFooter run={run} streamStatus="live" />);

    expect(screen.getByText("1m 30s")).toBeInTheDocument();
    expect(screen.getByText("1/2")).toBeInTheDocument();
    expect(screen.getByText("1.5k")).toBeInTheDocument();
    expect(screen.getByText("$0.0123")).toBeInTheDocument();
    expect(screen.getByText("Live")).toBeInTheDocument();
    expect(screen.getByText(formatClock(lastEventAt))).toBeInTheDocument();

    vi.useRealTimers();
  });

  it("uses a dash when the harness reported no cost", () => {
    render(
      <RunFooter
        run={makeRun({ status: "completed", finishedAt: "2026-01-01T12:00:10Z" })}
        streamStatus="ended"
      />,
    );

    expect(screen.getByText("Ended")).toBeInTheDocument();
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
