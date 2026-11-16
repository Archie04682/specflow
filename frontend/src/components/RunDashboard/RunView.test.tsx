import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FakeEventSource, lastSource } from "../../test/fakes";
import { graphNode, makeGraph, makeNode, makeRun } from "../../test/fixtures";
import { callBody, callUrl, fakeResponse, stubFetch } from "../../test/http";
import { useRunStore } from "../../stores/runStore";
import type { Run } from "../../types/run";
import { RunView } from "./RunView";

// React Flow needs layout APIs jsdom does not provide. The graph's own
// behavior is covered by layout and NodeCard tests; here the run view needs a
// rendered graph so node selection and details can be exercised.
vi.mock("./GraphPanel/WorkflowGraph", () => ({
  WorkflowGraph: ({ graph }: { graph: { nodes: Array<{ id: string }> } }) => (
    <div data-testid="graph">{graph.nodes.map((node) => node.id).join(",")}</div>
  ),
}));

function renderRun(run: Run, graph = makeGraph()) {
  const fetchMock = stubFetch((url) => {
    if (url.includes("/cancel")) return fakeResponse(200, { status: "cancelled" });
    if (url.includes("/retry")) return fakeResponse(200, { status: "running" });
    if (url.includes("/resume")) return fakeResponse(200, { status: "running" });
    if (url.includes("/graph")) return fakeResponse(200, graph);
    if (url.includes("/api/runs/run-1")) return fakeResponse(200, run);
    if (url.includes("/api/runs")) return fakeResponse(200, [run]);
    return fakeResponse(404, { error: "unexpected" });
  });

  const onNewRun = vi.fn();
  render(<RunView project="/repo" runId="run-1" onNewRun={onNewRun} />);
  return { fetchMock, onNewRun };
}

const terminalEvent = {
  seq: 20,
  at: "2026-01-01T12:02:00Z",
  runId: "run-1",
  type: "run.completed",
  data: { summary: "1/1 nodes completed in 2m" },
};

describe("RunView", () => {
  beforeEach(() => {
    FakeEventSource.reset();
    useRunStore.getState().resetRun();
    vi.stubGlobal("EventSource", FakeEventSource);
  });

  it("renders a completed run with its graph, stream, and final state", async () => {
    const run = makeRun({
      status: "completed",
      finishedAt: "2026-01-01T12:02:00Z",
      nodes: [makeNode({ status: "completed" })],
    });
    renderRun(run);

    expect(await screen.findByText("Standard feature")).toBeInTheDocument();
    expect(screen.getByTestId("graph")).toHaveTextContent("explore,propose");
    expect(
      screen.getByRole("separator", { name: "Resize activity panel" }),
    ).toBeInTheDocument();
    expect(screen.getByText("No events recorded")).toBeInTheDocument();

    await waitFor(() => expect(FakeEventSource.instances).toHaveLength(1));
    act(() => lastSource().open());
    act(() => lastSource().emit("run.completed", terminalEvent));

    expect((await screen.findAllByText("Ended")).length).toBeGreaterThan(0);
    expect(screen.getByText("Completed")).toBeInTheDocument();
  });

  it("shows per-node failure details and recovery controls for a failed run", async () => {
    const run = makeRun({
      status: "failed",
      error: "postcondition failed: every task must be checked",
      nodes: [
        makeNode({
          nodeId: "apply",
          title: "Apply",
          status: "failed",
          attempts: 1,
          error: "postcondition failed: every task must be checked",
          failureClass: "postcondition",
          checkpoint: "abcdef1234567890",
        }),
      ],
    });
    const graph = makeGraph({ nodes: [graphNode("apply")], edges: [] });
    renderRun(run, graph);

    await screen.findByText("Standard feature");
    expect(screen.getByRole("button", { name: "Resume run" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry Apply" })).toBeInTheDocument();

    act(() => useRunStore.getState().selectNode("apply"));

    expect(
      (await screen.findAllByText(/postcondition failed: every task/)).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("abcdef123456")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry clean" })).toBeInTheDocument();
  });

  it("cancels only after confirmation", async () => {
    const user = userEvent.setup();
    const run = makeRun({ status: "running" });
    const { fetchMock } = renderRun(run);

    await screen.findByText("Standard feature");
    await user.click(screen.getByRole("button", { name: "Cancel run" }));

    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText(/unfinished nodes are left unrun/)).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Cancel run" }));
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some((call) => String(call[0]).includes("/cancel")),
      ).toBe(true),
    );
  });

  it("retries a failed node with the forward mode", async () => {
    const user = userEvent.setup();
    const run = makeRun({
      status: "failed",
      nodes: [makeNode({ nodeId: "apply", title: "Apply", status: "failed" })],
    });
    const { fetchMock } = renderRun(run, makeGraph({ nodes: [graphNode("apply")], edges: [] }));

    await screen.findByText("Standard feature");
    await user.click(screen.getByRole("button", { name: "Retry Apply" }));

    await waitFor(() => {
      const retryIndex = fetchMock.mock.calls.findIndex((call) =>
        String(call[0]).includes("/retry"),
      );
      expect(retryIndex).toBeGreaterThanOrEqual(0);
    });

    const retryIndex = fetchMock.mock.calls.findIndex((call) =>
      String(call[0]).includes("/retry"),
    );
    expect(callUrl(fetchMock, retryIndex)).toContain("/api/runs/run-1/retry");
    expect(callBody(fetchMock, retryIndex)).toEqual({
      nodeId: "apply",
      mode: "forward",
    });
  });
});
