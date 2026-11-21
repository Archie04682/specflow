import { describe, expect, it } from "vitest";
import { ApiError } from "./client";
import {
  getProjectReadiness,
  installProjectSkills,
  seedProjectFlow,
} from "./onboarding";
import { callBody, callMethod, callUrl, fakeResponse, stubFetch } from "../test/http";

describe("onboarding api", () => {
  it("reads readiness for a project", async () => {
    const fetchMock = stubFetch(() =>
      fakeResponse(200, { project: "/repo", ready: false }),
    );

    const readiness = await getProjectReadiness("/repo");
    expect(callMethod(fetchMock)).toBe("GET");
    expect(callUrl(fetchMock)).toBe("/api/project/readiness?project=%2Frepo");
    expect(readiness.ready).toBe(false);
  });

  it("seeds the default flow with the project in the body", async () => {
    const fetchMock = stubFetch(() =>
      fakeResponse(200, {
        reference: "standard-feature",
        path: "/repo/.specflow/workflows/standard-feature.yaml",
      }),
    );

    const seeded = await seedProjectFlow("/repo");
    expect(callMethod(fetchMock)).toBe("POST");
    // A POST carries the project in the body, like run creation.
    expect(callUrl(fetchMock)).toBe("/api/project/flow");
    expect(callBody(fetchMock)).toEqual({
      project: "/repo",
      reference: undefined,
    });
    expect(seeded.reference).toBe("standard-feature");
  });

  it("asks to replace when skills conflict", async () => {
    const fetchMock = stubFetch(() =>
      fakeResponse(200, {
        installed: ["alpha"],
        unchanged: [],
        conflicts: [],
        missing: [],
      }),
    );

    await installProjectSkills("/repo", true);
    expect(callMethod(fetchMock)).toBe("POST");
    expect(callBody(fetchMock)).toEqual({ project: "/repo", replace: true });
  });

  it("surfaces a refused seed as an ApiError with the server message", async () => {
    stubFetch(() =>
      fakeResponse(409, { error: "flow already exists: standard-feature" }),
    );

    const error = await seedProjectFlow("/repo").catch((cause: unknown) => cause);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(409);
    expect((error as ApiError).message).toContain("already exists");
  });
});

describe("readiness normalization", () => {
  it("turns a null collection into an empty array", async () => {
    // The shape a non-repository read used to produce: harnesses null.
    stubFetch(() =>
      fakeResponse(200, {
        project: "/repo",
        git: { repository: false, hasCommit: false },
        layout: false,
        flows: null,
        skills: { roots: null, found: null },
        tools: { openspecFound: false, harnesses: null },
        uncommitted: null,
        actions: null,
        ready: false,
      }),
    );

    const readiness = await getProjectReadiness("/repo");
    expect(readiness.flows).toEqual([]);
    expect(readiness.skills.roots).toEqual([]);
    expect(readiness.skills.found).toEqual([]);
    expect(readiness.tools.harnesses).toEqual([]);
    expect(readiness.uncommitted).toEqual([]);
    expect(readiness.actions).toEqual([]);
  });
});
