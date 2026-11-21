import { describe, expect, it } from "vitest";
import { setupPlan } from "./onboardingSteps";
import type { ProjectReadiness } from "../../types/onboarding";

function readiness(overrides: Partial<ProjectReadiness> = {}): ProjectReadiness {
  return {
    project: "/repo",
    vcs: "git",
    vcsExplicit: false,
    vcsChangeable: false,
    git: { repository: true, branch: "main", hasCommit: true },
    layout: true,
    flows: [],
    skills: { roots: [], found: [] },
    tools: { openspecFound: true, harnesses: [] },
    uncommitted: [],
    secrets: [],
    offline: false,
    actions: [],
    ready: false,
    ...overrides,
  };
}

const flow = {
  reference: "standard-feature",
  path: "/repo/.specflow/workflows/standard-feature.yaml",
  valid: true,
  problems: [],
  requiredSkills: ["explore"],
  missingSkills: ["explore"],
};

describe("setupPlan", () => {
  it("orders the git steps and marks the current one", () => {
    const plan = setupPlan(readiness());

    expect(plan.steps.map((step) => step.id)).toEqual([
      "repository",
      "flow",
      "skills",
      "ready",
    ]);
    expect(plan.steps[0].state).toBe("done");
    expect(plan.steps[1].state).toBe("current");
    expect(plan.steps[2].state).toBe("pending");
    expect(plan.position).toBe(2);
  });

  it("skips the repository for a folder project", () => {
    const plan = setupPlan(readiness({ vcs: "folder", vcsExplicit: true }));

    expect(plan.steps.map((step) => step.id)).toEqual(["flow", "skills", "ready"]);
    expect(plan.position).toBe(1);
  });

  it("expands the first gap with its action detail", () => {
    const plan = setupPlan(
      readiness({
        git: { repository: true, branch: "main", hasCommit: false },
      }),
    );

    const repository = plan.steps[0];
    expect(repository.state).toBe("current");
    expect(repository.detail).toBe("No commits yet");
    expect(repository.hint).toContain("git commit");
  });

  it("states the blockers that have no action on the ready step", () => {
    const plan = setupPlan(
      readiness({
        flows: [flow],
        uncommitted: [".agents"],
      }),
    );

    const ready = plan.steps[3];
    expect(ready.notes).toEqual(["1 uncommitted path — a run needs a clean tree"]);
    expect(ready.state).toBe("pending");
  });

  it("is ready as a last step when everything checks out", () => {
    const plan = setupPlan(
      readiness({
        flows: [{ ...flow, missingSkills: [] }],
        ready: true,
      }),
    );

    expect(plan.steps.every((step) => step.state === "done")).toBe(true);
    expect(plan.position).toBe(4);
  });
});
