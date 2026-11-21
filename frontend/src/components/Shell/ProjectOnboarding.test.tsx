import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { refreshProjectData } from "../../lib/refresh";
import { ProjectOnboarding } from "./ProjectOnboarding";
import {
  ACTION_INIT_REPOSITORY,
  ACTION_INSTALL_SKILLS_LOCAL,
  ACTION_INSTALL_SKILLS_ONLINE,
  ACTION_SEED_FLOW,
  type ProjectReadiness,
} from "../../types/onboarding";
import { callBody, callMethod, fakeResponse, stubFetch } from "../../test/http";

vi.mock("../../lib/refresh", () => ({ refreshProjectData: vi.fn() }));

function makeReadiness(overrides: Partial<ProjectReadiness> = {}): ProjectReadiness {
  return {
    project: "/repo",
    vcs: "git",
    vcsExplicit: false,
    vcsChangeable: false,
    git: { repository: true, branch: "main", hasCommit: true },
    layout: false,
    flows: [],
    skills: { roots: [], found: [] },
    tools: {
      openspecFound: true,
      openspecVersion: "1.16.1",
      harnesses: [{ id: "pi", ready: true }],
    },
    uncommitted: [],
    secrets: [],
    offline: false,
    actions: [
      { name: ACTION_INIT_REPOSITORY, available: false, reason: "the project is already a git repository" },
      { name: ACTION_SEED_FLOW, available: true },
      { name: ACTION_INSTALL_SKILLS_LOCAL, available: false, reason: "no skills source configured" },
      { name: ACTION_INSTALL_SKILLS_ONLINE, available: true },
    ],
    ready: false,
    ...overrides,
  };
}

const seededFlow = {
  reference: "standard-feature",
  path: "/repo/.specflow/workflows/standard-feature.yaml",
  valid: true,
  problems: [],
  requiredSkills: ["openspec-explore", "openspec-propose"],
  missingSkills: ["openspec-explore", "openspec-propose"],
};

function renderCard(
  readiness: ProjectReadiness | null,
  handlers: { onProvisioned?: () => void } = {},
) {
  const onProvisioned = handlers.onProvisioned ?? vi.fn();
  const element = (next: ProjectReadiness | null) => (
    <ProjectOnboarding
      project="/repo"
      readiness={next}
      loading={false}
      onProvisioned={onProvisioned}
    />
  );
  const view = render(element(readiness));
  return {
    onProvisioned,
    rerenderWith: (next: ProjectReadiness | null) => view.rerender(element(next)),
  };
}

describe("ProjectOnboarding", () => {
  beforeEach(() => {
    vi.mocked(refreshProjectData).mockClear();
  });

  it("walks the gaps as numbered steps", () => {
    renderCard(makeReadiness());

    expect(screen.getByText("Step 2 of 4")).toBeInTheDocument();
    expect(screen.getByText("Repository")).toBeInTheDocument();
    expect(screen.getByText("On main with a commit")).toBeInTheDocument();
    expect(screen.getByText("No flow yet")).toBeInTheDocument();
    expect(screen.getByText("Waiting for a flow")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Add the standard flow" }),
    ).toBeInTheDocument();
  });

  it("marks a completed step and expands the next one", () => {
    renderCard(makeReadiness({ flows: [seededFlow as never] }));

    expect(screen.getByText("Step 3 of 4")).toBeInTheDocument();
    expect(screen.getByText("standard-feature")).toBeInTheDocument();
    expect(
      screen.getByText("Missing: openspec-explore, openspec-propose"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Install skills" })).toBeInTheDocument();
  });

  it("opens any step from the list", async () => {
    const user = userEvent.setup();
    renderCard(makeReadiness());

    await user.click(screen.getByRole("button", { name: "Ready" }));

    expect(screen.getByText("Step 4 of 4")).toBeInTheDocument();
  });

  it("moves between steps with Next and Back", async () => {
    const user = userEvent.setup();
    renderCard(makeReadiness({ flows: [seededFlow as never] }));

    expect(screen.getByText("Step 3 of 4")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Next: Ready" }));
    expect(screen.getByText("Step 4 of 4")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByText("Step 3 of 4")).toBeInTheDocument();
  });

  it("re-checks when work happened outside the app", async () => {
    const user = userEvent.setup();
    renderCard(
      makeReadiness({ git: { repository: true, branch: "main", hasCommit: false } }),
    );

    await user.click(screen.getByRole("button", { name: "Re-check" }));

    expect(refreshProjectData).toHaveBeenCalled();
  });

  it("renders nothing for a ready project", () => {
    renderCard(makeReadiness({ ready: true }));

    expect(
      screen.queryByRole("region", { name: "Project setup" }),
    ).not.toBeInTheDocument();
  });

  it("renders nothing while readiness has not arrived", () => {
    renderCard(null);

    expect(
      screen.queryByRole("region", { name: "Project setup" }),
    ).not.toBeInTheDocument();
  });

  it("summarises uncommitted work and offers to commit it", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((url) => {
      if (url === "/api/project/commit") {
        return fakeResponse(200, { commit: "abc123", paths: [".agents", "UPDATE.md"] });
      }
      return fakeResponse(404, { error: "unexpected" });
    });
    renderCard(
      makeReadiness({
        flows: [seededFlow as never],
        uncommitted: [".agents", "UPDATE.md"],
      }),
    );

    expect(
      screen.getByText("2 uncommitted paths — a run needs a clean tree"),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Ready" }));
    await user.click(screen.getByRole("button", { name: "Commit changes" }));

    expect(callBody(fetchMock)).toEqual({ project: "/repo" });
    expect(refreshProjectData).toHaveBeenCalled();
    expect(screen.getByText("Committed 2 paths.")).toBeInTheDocument();
  });

  it("refuses to commit secrets and offers to ignore them", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((url) => {
      if (url === "/api/project/ignore") {
        return fakeResponse(200, { ignored: [".env"] });
      }
      return fakeResponse(404, { error: "unexpected" });
    });
    renderCard(
      makeReadiness({
        flows: [seededFlow as never],
        uncommitted: [".env", "README.md"],
        secrets: [".env"],
      }),
    );

    expect(screen.getByText("These look like secrets: .env")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Ready" }));
    expect(
      screen.queryByRole("button", { name: "Commit changes" }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Add .env to .gitignore" }));

    expect(callBody(fetchMock)).toEqual({ project: "/repo", paths: [".env"] });
    expect(screen.getByText(/Ignored .env/)).toBeInTheDocument();
  });

  it("offers an initial commit when the repository has none", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((url) => {
      if (url === "/api/project/commit") {
        return fakeResponse(200, { commit: "abc123", paths: ["README.md"] });
      }
      return fakeResponse(404, { error: "unexpected" });
    });
    renderCard(
      makeReadiness({
        git: { repository: true, branch: "main", hasCommit: false },
        uncommitted: ["README.md"],
      }),
    );

    await user.click(screen.getByRole("button", { name: "Create initial commit" }));

    expect(callBody(fetchMock)).toEqual({ project: "/repo" });
    expect(refreshProjectData).toHaveBeenCalled();
  });

  it("seeds the flow, asks for a re-read, and keeps the remaining gap", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((url) => {
      if (url === "/api/project/flow") {
        return fakeResponse(200, seededFlow);
      }
      return fakeResponse(404, { error: "unexpected" });
    });
    const { onProvisioned, rerenderWith } = renderCard(makeReadiness());

    await user.click(screen.getByRole("button", { name: "Add the standard flow" }));

    expect(onProvisioned).toHaveBeenCalled();
    expect(refreshProjectData).toHaveBeenCalled();
    expect(callMethod(fetchMock)).toBe("POST");
    expect(callBody(fetchMock)).toEqual({ project: "/repo", reference: undefined });

    // The catalog now lists the seeded flow; its skills are still missing.
    rerenderWith(makeReadiness({ flows: [seededFlow as never] }));
    expect(screen.queryByText("No flow yet")).not.toBeInTheDocument();
    expect(screen.getByText("Step 3 of 4")).toBeInTheDocument();
    expect(
      screen.getByText("Missing: openspec-explore, openspec-propose"),
    ).toBeInTheDocument();
  });

  it("installs skills and reports what it wrote", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((url) => {
      if (url === "/api/project/skills") {
        return fakeResponse(200, {
          installed: ["openspec-explore"],
          unchanged: [],
          conflicts: [],
          missing: ["openspec-propose"],
          output: "cloned",
        });
      }
      return fakeResponse(404, { error: "unexpected" });
    });
    renderCard(
      makeReadiness({
        flows: [seededFlow as never],
        actions: [
          { name: ACTION_SEED_FLOW, available: false, reason: "the default flow already exists" },
          { name: ACTION_INSTALL_SKILLS_LOCAL, available: true },
          { name: ACTION_INSTALL_SKILLS_ONLINE, available: false, reason: "a local skills source is configured" },
        ],
      }),
    );

    await user.click(screen.getByRole("button", { name: "Install skills from source" }));

    expect(screen.getByText("Installed: openspec-explore")).toBeInTheDocument();
    expect(screen.getByText("Still missing: openspec-propose")).toBeInTheDocument();
    expect(refreshProjectData).toHaveBeenCalled();
    expect(callBody(fetchMock)).toEqual({ project: "/repo", replace: false });
  });

  it("offers to initialise a project that is not a repository", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((url) => {
      if (url === "/api/project/repository") {
        return fakeResponse(200, { path: "/repo", branch: "main" });
      }
      return fakeResponse(404, { error: "unexpected" });
    });
    renderCard(
      makeReadiness({
        git: { repository: false, hasCommit: false, reason: "not a git repository" },
        actions: [
          { name: ACTION_INIT_REPOSITORY, available: true },
          { name: ACTION_SEED_FLOW, available: true },
          { name: ACTION_INSTALL_SKILLS_LOCAL, available: true },
          { name: ACTION_INSTALL_SKILLS_ONLINE, available: false, reason: "a local skills source is configured" },
        ],
      }),
    );

    await user.click(screen.getByRole("button", { name: "Initialize repository" }));

    expect(callBody(fetchMock)).toEqual({ project: "/repo" });
    expect(refreshProjectData).toHaveBeenCalled();
  });

  it("asks for the first commit in an initialised repository", () => {
    renderCard(
      makeReadiness({ git: { repository: true, branch: "main", hasCommit: false } }),
    );

    expect(screen.getByText("No commits yet")).toBeInTheDocument();
    expect(
      screen.getByText('git add -A && git commit -m "Initial commit"'),
    ).toBeInTheDocument();
  });

  it("names a detached HEAD as a gap", () => {
    renderCard(
      makeReadiness({
        git: {
          repository: true,
          hasCommit: true,
          reason: "project has a detached HEAD",
        },
      }),
    );

    expect(screen.getByText("No branch checked out")).toBeInTheDocument();
    expect(screen.getByText("project has a detached HEAD")).toBeInTheDocument();
  });

  it("never renders an empty card for a project the server calls unready", () => {
    renderCard(
      makeReadiness({ flows: [{ ...seededFlow, missingSkills: [] } as never] }),
    );

    expect(
      screen.getByText("The server reports a gap this build cannot name."),
    ).toBeInTheDocument();
  });

  it("explains what offline mode withholds", () => {
    renderCard(
      makeReadiness({
        offline: true,
        flows: [seededFlow as never],
        actions: [
          { name: ACTION_SEED_FLOW, available: false, reason: "the default flow already exists" },
          {
            name: ACTION_INSTALL_SKILLS_LOCAL,
            available: false,
            reason: "offline mode: configure a skills source",
          },
          { name: ACTION_INSTALL_SKILLS_ONLINE, available: false, reason: "offline mode" },
        ],
      }),
    );

    expect(
      screen.getByText("offline mode: configure a skills source"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Install skills/ }),
    ).not.toBeInTheDocument();
  });

  it("offers the local install when offline mode has a source", () => {
    renderCard(
      makeReadiness({
        offline: true,
        skillsSource: "/srv/skills",
        flows: [seededFlow as never],
        actions: [
          { name: ACTION_SEED_FLOW, available: false, reason: "the default flow already exists" },
          { name: ACTION_INSTALL_SKILLS_LOCAL, available: true },
          { name: ACTION_INSTALL_SKILLS_ONLINE, available: false, reason: "offline mode" },
        ],
      }),
    );

    expect(
      screen.getByRole("button", { name: "Install skills from source" }),
    ).toBeInTheDocument();
  });

  it("records the folder mode, collapses the choice, and drops the git step", async () => {
    const user = userEvent.setup();
    const fetchMock = stubFetch((url) => {
      if (url === "/api/project/setup") {
        return fakeResponse(200, { vcs: "folder", repository: false, hasCommit: false });
      }
      return fakeResponse(404, { error: "unexpected" });
    });
    const { rerenderWith } = renderCard(
      makeReadiness({
        vcsChangeable: true,
        git: { repository: false, hasCommit: false, reason: "not a git repository" },
        actions: [{ name: ACTION_INIT_REPOSITORY, available: true }],
      }),
    );

    await user.click(screen.getByRole("button", { name: "Just a folder" }));

    expect(callBody(fetchMock)).toEqual({ project: "/repo", vcs: "folder" });
    expect(refreshProjectData).toHaveBeenCalled();

    rerenderWith(
      makeReadiness({
        vcs: "folder",
        vcsExplicit: true,
        vcsChangeable: true,
        git: { repository: false, hasCommit: false, reason: "not a git repository" },
        flows: [seededFlow as never],
        actions: [
          { name: ACTION_INIT_REPOSITORY, available: false, reason: "the project uses a folder workspace" },
          { name: ACTION_SEED_FLOW, available: false, reason: "the default flow already exists" },
          { name: ACTION_INSTALL_SKILLS_LOCAL, available: true },
        ],
      }),
    );
    expect(screen.queryByText("Repository")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Just a folder" })).not.toBeInTheDocument();
    expect(screen.getByText("Workspace: a copy of this folder")).toBeInTheDocument();
  });

  it("collapses the mode choice once a mode is recorded", () => {
    renderCard(makeReadiness({ vcsExplicit: true, vcsChangeable: true }));

    expect(screen.getByText("Workspace: a git worktree")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Use git" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Just a folder" })).not.toBeInTheDocument();
  });

  it("offers the mode choice while none is recorded", () => {
    renderCard(makeReadiness({ vcsChangeable: true }));

    expect(screen.getByText("Where should a run work?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Use git" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Just a folder" })).toBeInTheDocument();
  });
});
