import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { makeRun, makeWorkflow } from "../../test/fixtures";
import { fakeResponse, stubFetch } from "../../test/http";
import type { Run } from "../../types/run";
import type { WorkflowSummary } from "../../types/graph";
import { ControlPanel } from "./ControlPanel";
import { ShellChromeProvider } from "./ShellChrome";

interface Options {
  projects?: string[];
  activeProject?: string;
  runs?: Record<string, Run[]>;
  workflows?: Record<string, WorkflowSummary[]>;
}

function stubCatalogs(options: Options = {}) {
  return stubFetch((url) => {
    const parsed = new URL(String(url), "http://localhost");
    const project = parsed.searchParams.get("project") ?? "";
    if (parsed.pathname === "/api/runs") {
      return fakeResponse(200, options.runs?.[project] ?? []);
    }
    if (parsed.pathname === "/api/workflows") {
      return fakeResponse(200, { workflows: options.workflows?.[project] ?? [] });
    }
    if (parsed.pathname === "/api/project/repository") {
      return fakeResponse(200, {
        repository: true,
        branch: "main",
        hasCommit: true,
        vcs: "git",
      });
    }
    return fakeResponse(404, { error: "unexpected" });
  });
}

function renderPanel(options: Options = {}) {
  const navigate = vi.fn();
  const onActivateProject = vi.fn();
  const onNewChat = vi.fn();
  const onOpenSettings = vi.fn();
  render(
    <ShellChromeProvider
      value={{
        panelCollapsed: false,
        panelHidden: false,
        openPanel: () => {},
        togglePanel: () => {},
      }}
    >
      <ControlPanel
        projects={options.projects ?? ["/work/repo", "/personal/repo"]}
        activeProject={options.activeProject ?? "/work/repo"}
        route={{ view: "home" }}
        liveRun={null}
        navigate={navigate}
        onActivateProject={onActivateProject}
        onNewChat={onNewChat}
        onOpenSettings={onOpenSettings}
      />
    </ShellChromeProvider>,
  );
  return { navigate, onActivateProject, onNewChat, onOpenSettings };
}

describe("ControlPanel", () => {
  it("renders the active project with its Chats and Flows leaves", async () => {
    stubCatalogs({
      runs: {
        "/work/repo": [makeRun({ id: "work-run", project: "/work/repo" })],
      },
      workflows: { "/work/repo": [makeWorkflow()] },
    });
    renderPanel();

    expect(
      await screen.findByRole("button", { name: "repo (work)" }),
    ).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Chats")).toBeInTheDocument();
    expect(screen.getByText("Flows")).toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: /Open chat work-run/ }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("button", {
        name: "Open flow Standard feature",
      }),
    ).toBeInTheDocument();
  });

  it("offers to create a repository instead of chats and flows", async () => {
    const user = userEvent.setup();
    let isRepository = false;
    const fetchMock = stubFetch((url, init) => {
      const parsed = new URL(String(url), "http://localhost");
      const project = parsed.searchParams.get("project") ?? "";
      if (parsed.pathname === "/api/project/repository") {
        if (init?.method === "POST") {
          isRepository = true;
          return fakeResponse(200, { path: project, branch: "main" });
        }
        return fakeResponse(
          200,
          isRepository
            ? { repository: true, branch: "main", hasCommit: false }
            : { repository: false, hasCommit: false, reason: "not a git repository" },
        );
      }
      if (parsed.pathname === "/api/runs") {
        return fakeResponse(200, []);
      }
      if (parsed.pathname === "/api/workflows") {
        return fakeResponse(200, { workflows: [] });
      }
      return fakeResponse(404, { error: "unexpected" });
    });

    renderPanel({ projects: ["/plain"], activeProject: "/plain" });

    expect(
      await screen.findByRole("button", { name: "Create git repository" }),
    ).toBeInTheDocument();
    // Two sections that could only report an error are not rendered.
    expect(screen.queryByText("Chats")).not.toBeInTheDocument();
    expect(screen.queryByText("Flows")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Create git repository" }));

    // Creating the repository refreshes the project reads, so the leaves take
    // the action's place.
    expect(await screen.findByText("Chats")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Create git repository" }),
    ).not.toBeInTheDocument();

    const posted = fetchMock.mock.calls.some(
      (call) =>
        String(call[0]).includes("/api/project/repository") &&
        (call[1] as RequestInit | undefined)?.method === "POST",
    );
    expect(posted).toBe(true);
  });

  it("keeps the leaves for a folder project without a repository", async () => {
    stubFetch((url) => {
      const parsed = new URL(String(url), "http://localhost");
      if (parsed.pathname === "/api/project/repository") {
        return fakeResponse(200, {
          repository: false,
          hasCommit: false,
          reason: "not a git repository",
          vcs: "folder",
        });
      }
      if (parsed.pathname === "/api/runs") {
        return fakeResponse(200, []);
      }
      if (parsed.pathname === "/api/workflows") {
        return fakeResponse(200, { workflows: [] });
      }
      return fakeResponse(404, { error: "unexpected" });
    });

    renderPanel({ projects: ["/plain"], activeProject: "/plain" });

    expect(await screen.findByText("Chats")).toBeInTheDocument();
    expect(screen.getByText("Flows")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Create git repository" }),
    ).not.toBeInTheDocument();
  });

  it("collapses and expands a project node", async () => {
    const user = userEvent.setup();
    stubCatalogs();
    renderPanel();
    const project = await screen.findByRole("button", {
      name: "repo (work)",
    });

    await user.click(project);
    expect(project).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Chats")).not.toBeInTheDocument();

    await user.click(project);
    expect(await screen.findByText("Chats")).toBeInTheDocument();
  });

  it("activates a project when its row is toggled", async () => {
    const user = userEvent.setup();
    stubCatalogs();
    const { onActivateProject } = renderPanel();
    const other = await screen.findByRole("button", {
      name: "repo (personal)",
    });
    await user.click(other);
    expect(onActivateProject).toHaveBeenCalledWith("/personal/repo");
  });

  it("opens runs and flows with their project", async () => {
    const user = userEvent.setup();
    stubCatalogs({
      runs: { "/work/repo": [makeRun({ id: "w-run", project: "/work/repo" })] },
      workflows: { "/work/repo": [makeWorkflow()] },
    });
    const { navigate } = renderPanel();

    await user.click(
      await screen.findByRole("button", { name: /Open chat w-run/ }),
    );
    expect(navigate).toHaveBeenCalledWith({
      view: "chat",
      runId: "w-run",
      project: "/work/repo",
    });

    await user.click(
      await screen.findByRole("button", {
        name: "Open flow Standard feature",
      }),
    );
    expect(navigate).toHaveBeenCalledWith({
      view: "flow",
      reference: "standard",
      project: "/work/repo",
    });
  });

  it("starts chats and flows in a project from the leaf actions", async () => {
    const user = userEvent.setup();
    stubCatalogs();
    const { navigate, onActivateProject } = renderPanel();

    await user.click(
      await screen.findByRole("button", {
        name: "New chat in repo (work)",
      }),
    );
    expect(onActivateProject).toHaveBeenCalledWith("/work/repo");
    expect(navigate).toHaveBeenCalledWith({ view: "home" });

    await user.click(
      await screen.findByRole("button", {
        name: "New flow in repo (work)",
      }),
    );
    expect(navigate).toHaveBeenCalledWith({
      view: "flow",
      project: "/work/repo",
    });
  });

  it("marks the current chat and flow rows", async () => {
    stubCatalogs({
      runs: { "/work/repo": [makeRun({ id: "run-cur", project: "/work/repo" })] },
      workflows: { "/work/repo": [makeWorkflow()] },
    });
    const navigate = vi.fn();
    render(
      <ControlPanel
        projects={["/work/repo"]}
        activeProject="/work/repo"
        route={{ view: "chat", runId: "run-cur", project: "/work/repo" }}
        liveRun={null}
        navigate={navigate}
        onActivateProject={() => {}}
        onNewChat={() => {}}
        onOpenSettings={() => {}}
      />,
    );

    const run = await screen.findByRole("button", {
      name: /Open chat run-cur/,
    });
    expect(run).toHaveAttribute("aria-current", "true");
  });

  it("shows no-projects state and opens settings", async () => {
    const user = userEvent.setup();
    stubCatalogs();
    const { onOpenSettings } = renderPanel({ projects: [], activeProject: "" });

    expect(screen.getByText("No projects yet")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add project" }));
    expect(onOpenSettings).toHaveBeenCalledTimes(1);
  });

  it("opens settings from the footer", async () => {
    const user = userEvent.setup();
    stubCatalogs();
    const { onOpenSettings } = renderPanel();
    await user.click(screen.getByRole("button", { name: "Settings" }));
    expect(onOpenSettings).toHaveBeenCalledTimes(1);
  });

  it("keeps the top New chat action separate from the project leaves", async () => {
    const user = userEvent.setup();
    stubCatalogs();
    const { onNewChat } = renderPanel();
    await user.click(screen.getByRole("button", { name: "New chat" }));
    expect(onNewChat).toHaveBeenCalledTimes(1);
  });
});
