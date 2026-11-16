import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useRunStore } from "../../stores/runStore";
import { fakeResponse, stubFetch } from "../../test/http";
import { makeHarness, makeRun } from "../../test/fixtures";
import type { WorkflowSummary } from "../../types/graph";
import { ChatHome } from "./ChatHome";

function catalog(): WorkflowSummary[] {
  return [
    {
      reference: "standard",
      path: "/repo/.specflow/workflows/standard.yaml",
      name: "Standard feature",
      description: "The canonical lifecycle.",
      version: 1,
      valid: true,
      problems: [],
      nodes: [
        {
          id: "explore",
          title: "Explore",
          kind: "agent",
          interactive: true,
          allowInject: true,
          requireApproval: false,
        },
      ],
      edges: [],
    },
    {
      reference: "broken",
      path: "/repo/workflows/broken.yaml",
      name: "Broken",
      version: 0,
      valid: false,
      problems: ['duplicate node id "a"'],
      nodes: [],
      edges: [],
    },
  ];
}

function renderHome(overrides: Partial<Parameters<typeof ChatHome>[0]> = {}) {
  const handlers = {
    onReloadWorkflows: vi.fn(),
    onStarted: vi.fn(),
    onNewFlow: vi.fn(),
    onOpenProject: vi.fn(),
    onProjectChange: vi.fn(),
  };
  render(
    <ChatHome
      project="/repo"
      workflows={catalog()}
      workflowsLoading={false}
      workflowsError={null}
      {...handlers}
      {...overrides}
    />,
  );
  return handlers;
}

describe("ChatHome", () => {
  beforeEach(() => {
    useRunStore.getState().resetRun();
  });

  it("renders the composer with the catalog, disabled invalid flows, and suggestions", async () => {
    stubFetch((url) => {
      if (url.includes("/api/harnesses")) {
        return fakeResponse(200, { harnesses: [makeHarness()] });
      }
      return fakeResponse(404, { error: "unexpected" });
    });

    renderHome();

    const select = screen.getByLabelText("Flow");
    await waitFor(() => expect(select).toHaveValue("standard"));
    expect(screen.getByRole("option", { name: /broken/i })).toBeDisabled();
    expect(screen.getByText(/1 flow cannot be used/)).toBeInTheDocument();
    expect(screen.getByText(/duplicate node id/)).toBeInTheDocument();
  });

  it("shows a not-ready harness as a warning without blocking the form", async () => {
    stubFetch((url) => {
      if (url.includes("/api/harnesses")) {
        return fakeResponse(200, {
          harnesses: [makeHarness({ ready: false, readyError: "no credentials" })],
        });
      }
      return fakeResponse(404, { error: "unexpected" });
    });

    renderHome();
    expect(await screen.findByText(/Harness not ready/i)).toBeInTheDocument();
    expect(screen.getByText(/no credentials/)).toBeInTheDocument();
    expect(screen.getByLabelText("Objective")).toBeEnabled();
  });

  it("starts a run with the selected flow and objective", async () => {
    const user = userEvent.setup();
    const created = makeRun({ id: "run-42", status: "running" });
    const fetchMock = stubFetch((url, init) => {
      if (url.includes("/api/harnesses")) {
        return fakeResponse(200, { harnesses: [makeHarness()] });
      }
      if (url === "/api/runs" && init?.method === "POST") {
        return fakeResponse(201, created);
      }
      return fakeResponse(404, { error: "unexpected" });
    });

    const { onStarted } = renderHome();
    await waitFor(() => expect(screen.getByLabelText("Flow")).toHaveValue("standard"));
    await user.type(screen.getByLabelText("Objective"), "Add an export endpoint");
    await user.click(screen.getByRole("button", { name: "Start run" }));

    const post = fetchMock.mock.calls.find((call) => String(call[0]) === "/api/runs");
    expect(post).toBeDefined();
    expect(JSON.parse(String((post![1] as RequestInit).body))).toEqual({
      project: "/repo",
      workflow: "standard",
      objective: "Add an export endpoint",
    });
    expect(onStarted).toHaveBeenCalledWith("run-42");
  });

  it("keeps the draft and shows the server error when creation is rejected", async () => {
    const user = userEvent.setup();
    stubFetch((url, init) => {
      if (url.includes("/api/harnesses")) {
        return fakeResponse(200, { harnesses: [makeHarness()] });
      }
      if (url === "/api/runs" && init?.method === "POST") {
        return fakeResponse(400, {
          error: "project has uncommitted changes: main.go",
        });
      }
      return fakeResponse(404, { error: "unexpected" });
    });

    const { onStarted } = renderHome();
    await waitFor(() => expect(screen.getByLabelText("Flow")).toHaveValue("standard"));
    await user.type(screen.getByLabelText("Objective"), "Add an export endpoint");
    await user.click(screen.getByRole("button", { name: "Start run" }));

    expect(
      await screen.findByText(/project has uncommitted changes/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Objective")).toHaveValue(
      "Add an export endpoint",
    );
    expect(onStarted).not.toHaveBeenCalled();
  });

  it("offers New flow when the project has no usable flow", async () => {
    const user = userEvent.setup();
    stubFetch((url) => {
      if (url.includes("/api/harnesses")) {
        return fakeResponse(200, { harnesses: [makeHarness()] });
      }
      return fakeResponse(404, { error: "unexpected" });
    });

    const { onNewFlow } = renderHome({
      workflows: [
        {
          ...catalog()[1]!,
          reference: "broken-only",
          valid: false,
        },
      ],
    });

    expect(screen.getByLabelText("Flow")).toBeDisabled();
    expect(screen.getByText("No flows available")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "New flow" }));
    expect(onNewFlow).toHaveBeenCalledWith("/repo");
    expect(screen.getByText("Create a flow to start a run.")).toBeInTheDocument();
  });

  it("asks for a project before composing a run", () => {
    stubFetch(() => fakeResponse(200, { harnesses: [] }));
    renderHome({ project: "" });

    expect(
      screen.getByRole("heading", { name: /Choose a project to begin/ }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Objective")).not.toBeInTheDocument();
  });
});
