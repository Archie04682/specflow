import { describe, expect, it } from "vitest";
import {
  addProject,
  hasProject,
  normalizeProjects,
  projectLabels,
  removeProject,
} from "./projects";

describe("project registry", () => {
  it("trims, drops empties, and dedupes while keeping order", () => {
    expect(
      normalizeProjects(["/b", " /a ", "", "/b", "   ", "/c"]),
    ).toEqual(["/b", "/a", "/c"]);
  });

  it("adds a project once", () => {
    expect(addProject(["/a"], "/b")).toEqual(["/a", "/b"]);
    expect(addProject(["/a"], "/a")).toEqual(["/a"]);
    expect(hasProject(["/a"], "/a")).toBe(true);
    expect(hasProject(["/a"], "/b")).toBe(false);
  });

  it("removes a project by exact path", () => {
    expect(removeProject(["/a", "/b"], "/b")).toEqual(["/a"]);
    expect(removeProject(["/a"], "/missing")).toEqual(["/a"]);
  });

  it("labels projects by basename", () => {
    expect(projectLabels(["/code/alpha", "/code/beta"])).toEqual([
      { path: "/code/alpha", name: "alpha", suffix: undefined },
      { path: "/code/beta", name: "beta", suffix: undefined },
    ]);
  });

  it("disambiguates duplicate basenames with the fewest parent segments", () => {
    expect(projectLabels(["/work/specflow", "/personal/specflow"])).toEqual([
      { path: "/work/specflow", name: "specflow", suffix: "work" },
      { path: "/personal/specflow", name: "specflow", suffix: "personal" },
    ]);

    expect(
      projectLabels(["/work/apps/specflow", "/work/tools/specflow"]),
    ).toEqual([
      { path: "/work/apps/specflow", name: "specflow", suffix: "apps" },
      { path: "/work/tools/specflow", name: "specflow", suffix: "tools" },
    ]);
  });

  it("widens the suffix when parent names also collide", () => {
    expect(
      projectLabels(["/one/work/specflow", "/two/work/specflow"]),
    ).toEqual([
      { path: "/one/work/specflow", name: "specflow", suffix: "one/work" },
      { path: "/two/work/specflow", name: "specflow", suffix: "two/work" },
    ]);
  });
});
