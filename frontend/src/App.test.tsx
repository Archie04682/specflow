import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { FakeEventSource } from "./test/fakes";
import { makeGraph, makeHarness, makeNode, makeRun, makeWorkflow } from "./test/fixtures";
import { fakeResponse, stubFetch } from "./test/http";
import { useRunStore } from "./stores/runStore";
import type { Run } from "./types/run";
import type { WorkflowSummary } from "./types/graph";

vi.mock("./components/RunDashboard/GraphPanel/WorkflowGraph", () => ({
  WorkflowGraph: () => <div data-testid="graph" />,
}));

vi.mock("./components/WorkflowEditor/EditorGraph", () => ({
  EditorGraph: () => <div data-testid="editor-graph" />,
}));

const source = {
  reference: "standard",
  path: "/repo/.specflow/workflows/standard.yaml",
  source: "id: standard\n",
  hash: "hash-1",
  isTemplate: false,
  draft: {
    id: "standard",
    name: "Standard feature",
    version: 1,
    defaults: { harness: "pi" },
    nodes: [{ id: "explore", title: "Explore", kind: "agent" }],
    edges: [],
  },
};

interface StubOptions {
  runs?: Record<string, Run[]>;
  workflows?: Record<string, WorkflowSummary[]>;
  run?: Run;
}

function stubApp({ runs = {}, workflows = {}, run }: StubOptions = {}) {
  return stubFetch((url) => {
    const parsed = new URL(String(url), "http://localhost");
    const path = parsed.pathname;
    const project = parsed.searchParams.get("project") ?? "";
    if (path.includes("/api/harnesses")) {
      return fakeResponse(200, { harnesses: [makeHarness()] });
    }
    if (path.includes("/api/skills")) return fakeResponse(200, { skills: [] });
    if (path.endsWith("/graph")) return fakeResponse(200, makeGraph());
    if (/^\/api\/runs\/[^/]+$/.test(path)) {
      return run ? fakeResponse(200, run) : fakeResponse(404, { error: "missing run" });
    }
    if (path === "/api/runs") return fakeResponse(200, runs[project] ?? []);
    if (path === "/api/workflows") {
      return fakeResponse(200, { workflows: workflows[project] ?? [] });
    }
    if (path.startsWith("/api/workflows/")) return fakeResponse(200, source);
    return fakeResponse(404, { error: "unexpected" });
  });
}

function setProjects(projects: string[], active: string) {
  localStorage.setItem("specflow.projects", JSON.stringify(projects));
  localStorage.setItem("specflow.project", active);
}

describe("App shell", () => {
  beforeEach(() => {
    useRunStore.getState().resetRun();
    localStorage.clear();
    window.history.replaceState(null, "", "/");
    FakeEventSource.reset();
    vi.stubGlobal("EventSource", FakeEventSource);
  });

  it("renders the project tree and the chat home", async () => {
    setProjects(["/repo"], "/repo");
    stubApp({
      runs: { "/repo": [makeRun({ project: "/repo", status: "completed", objective: "Fix the flaky test" })] },
      workflows: { "/repo": [makeWorkflow()] },
    });
    render(<App />);

    expect(
      await screen.findByRole("heading", { name: "New chat", level: 1 }),
    ).toBeInTheDocument();
    const panel = screen.getByRole("complementary");
    expect(
      within(panel).getByRole("button", { name: "repo" }),
    ).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Chats")).toBeInTheDocument();
    expect(screen.getByText("Flows")).toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: /Open chat/ }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: "Open flow Standard feature" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Settings" })).toBeInTheDocument();
    expect(screen.getByLabelText("Flow")).toBeInTheDocument();
    expect(screen.getByLabelText("Objective")).toBeInTheDocument();
  });

  it("collapses and expands a project in the tree", async () => {
    const user = userEvent.setup();
    setProjects(["/repo"], "/repo");
    stubApp({ runs: { "/repo": [makeRun({ project: "/repo" })] } });
    render(<App />);

    const panel = screen.getByRole("complementary");
    const project = await within(panel).findByRole("button", { name: "repo" });
    expect(project).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Chats")).toBeInTheDocument();

    await user.click(project);
    expect(project).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Chats")).not.toBeInTheDocument();
    expect(screen.queryByText("Flows")).not.toBeInTheDocument();

    await user.click(project);
    expect(await screen.findByText("Chats")).toBeInTheDocument();
  });

  it("adds a project from settings and activates it", async () => {
    const user = userEvent.setup();
    setProjects(["/repo"], "/repo");
    stubApp();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "Settings" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Projects")).toBeInTheDocument();

    await user.type(
      within(dialog).getByLabelText("Project"),
      "/other",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Add project" }),
    );

    await waitFor(() =>
      expect(localStorage.getItem("specflow.project")).toBe("/other"),
    );
    expect(JSON.parse(localStorage.getItem("specflow.projects") ?? "[]")).toEqual([
      "/repo",
      "/other",
    ]);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(
      await within(screen.getByRole("complementary")).findByRole("button", {
        name: "other",
      }),
    ).toBeInTheDocument();
  });

  it("removes a project from settings and falls back to another", async () => {
    const user = userEvent.setup();
    setProjects(["/repo", "/other"], "/other");
    stubApp();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "Settings" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", {
        name: "Remove other from the workbench",
      }),
    );

    expect(JSON.parse(localStorage.getItem("specflow.projects") ?? "[]")).toEqual([
      "/repo",
    ]);
    expect(localStorage.getItem("specflow.project")).toBe("/repo");
    expect(
      within(dialog).queryByRole("button", { name: /Use project other/ }),
    ).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("asks for a project when a run deep link has none", async () => {
    window.history.replaceState(null, "", "/?run=run-1");
    stubApp();
    render(<App />);

    expect(
      await screen.findByRole("heading", {
        name: /Which project holds this run/i,
      }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add project" })).toBeInTheDocument();
  });

  it("keeps a run deep link while its project is supplied", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/?run=run-1");
    const run = makeRun({ status: "completed", nodes: [makeNode({ status: "completed" })] });
    stubApp({ run });

    render(<App />);
    await screen.findByRole("heading", { name: /Which project holds this run/i });

    await user.type(screen.getByLabelText("Project"), "/repo");
    await user.click(screen.getByRole("button", { name: "Open run" }));

    expect(await screen.findByText("Standard feature")).toBeInTheDocument();
    expect(window.location.search).toBe("?run=run-1");
    expect(localStorage.getItem("specflow.project")).toBe("/repo");
  });

  it("opens a project-scoped run and adopts its project", async () => {
    setProjects(["/repo"], "/repo");
    window.history.replaceState(null, "", "/?run=run-1&project=%2Fother");
    const run = makeRun({
      project: "/other",
      status: "completed",
      nodes: [makeNode({ status: "completed" })],
    });
    stubApp({ run });
    render(<App />);

    expect(await screen.findByText("Standard feature")).toBeInTheDocument();
    await waitFor(() =>
      expect(localStorage.getItem("specflow.project")).toBe("/other"),
    );
    expect(JSON.parse(localStorage.getItem("specflow.projects") ?? "[]")).toContain(
      "/other",
    );
  });

  it("opens the new-flow editor scoped to the active project", async () => {
    setProjects(["/repo"], "/repo");
    window.history.replaceState(null, "", "/?view=editor");
    stubApp();
    render(<App />);

    expect(
      await screen.findByRole("heading", { name: "New flow", level: 1 }),
    ).toBeInTheDocument();
    expect(await screen.findByTestId("editor-graph")).toBeInTheDocument();
    expect(localStorage.getItem("specflow.project")).toBe("/repo");
  });

  it("opens a flow from the project tree", async () => {
    const user = userEvent.setup();
    setProjects(["/repo"], "/repo");
    stubApp({ workflows: { "/repo": [makeWorkflow()] } });
    render(<App />);

    const flow = await screen.findByRole("button", {
      name: "Open flow Standard feature",
    });
    await user.click(flow);

    expect(
      await screen.findByRole("heading", { name: "Standard feature", level: 1 }),
    ).toBeInTheDocument();
    expect(window.location.search).toContain("project=%2Frepo");
  });

  it("treats old flows and history links as the chat home", async () => {
    setProjects(["/repo"], "/repo");
    window.history.replaceState(null, "", "/?view=flows");
    stubApp();
    render(<App />);
    expect(
      await screen.findByRole("heading", { name: "New chat", level: 1 }),
    ).toBeInTheDocument();

    act(() => {
      window.history.replaceState(null, "", "/?view=history");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(
      await screen.findByRole("heading", { name: "New chat", level: 1 }),
    ).toBeInTheDocument();
  });

  it("collapses the panel, persists it, and restores it from the header", async () => {
    const user = userEvent.setup();
    setProjects(["/repo"], "/repo");
    stubApp();
    const first = render(<App />);

    await screen.findByRole("heading", { name: "New chat", level: 1 });
    await user.click(
      screen.getByRole("button", { name: "Collapse control panel" }),
    );
    expect(localStorage.getItem("specflow.panelCollapsed")).toBe("1");
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
    first.unmount();

    render(<App />);
    await screen.findByRole("heading", { name: "New chat", level: 1 });
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Show control panel" }));
    expect(screen.getByRole("complementary")).toBeInTheDocument();
    expect(localStorage.getItem("specflow.panelCollapsed")).toBeNull();
  });
});
