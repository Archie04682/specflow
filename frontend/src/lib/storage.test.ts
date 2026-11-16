import { beforeEach, describe, expect, it } from "vitest";
import {
  readStoredPanelCollapsed,
  readStoredProjectState,
  readStoredRunSplit,
  writeStoredActiveProject,
  writeStoredPanelCollapsed,
  writeStoredProjects,
  writeStoredRunSplit,
} from "./storage";
import { MAX_RUN_SIDE, MIN_RUN_SIDE } from "./split";

describe("project storage", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("migrates the legacy single project into the list", () => {
    localStorage.setItem("specflow.project", "/repo");
    expect(readStoredProjectState()).toEqual({
      projects: ["/repo"],
      activeProject: "/repo",
    });
  });

  it("reads the list and keeps the active project", () => {
    writeStoredProjects(["/a", "/b"]);
    writeStoredActiveProject("/b");
    expect(readStoredProjectState()).toEqual({
      projects: ["/a", "/b"],
      activeProject: "/b",
    });
  });

  it("keeps a stored active path that is missing from the list", () => {
    writeStoredProjects(["/a", "/b"]);
    writeStoredActiveProject("/gone");
    expect(readStoredProjectState()).toEqual({
      projects: ["/a", "/b", "/gone"],
      activeProject: "/gone",
    });
  });

  it("falls back to the first project when no active path is stored", () => {
    writeStoredProjects(["/a", "/b"]);
    expect(readStoredProjectState().activeProject).toBe("/a");
  });

  it("survives malformed project JSON", () => {
    localStorage.setItem("specflow.projects", "{not json");
    localStorage.setItem("specflow.project", "/repo");
    expect(readStoredProjectState()).toEqual({
      projects: ["/repo"],
      activeProject: "/repo",
    });
  });
});

describe("panel storage", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("round-trips the collapsed flag", () => {
    expect(readStoredPanelCollapsed()).toBe(false);
    writeStoredPanelCollapsed(true);
    expect(readStoredPanelCollapsed()).toBe(true);
    writeStoredPanelCollapsed(false);
    expect(readStoredPanelCollapsed()).toBe(false);
  });
});

describe("run split storage", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("round-trips a chosen width", () => {
    expect(readStoredRunSplit()).toBeNull();
    writeStoredRunSplit(640);
    expect(readStoredRunSplit()).toBe(640);
  });

  it("clamps a stored width into the allowed range", () => {
    localStorage.setItem("specflow.runSplit", "20");
    expect(readStoredRunSplit()).toBe(MIN_RUN_SIDE);
    localStorage.setItem("specflow.runSplit", "99999");
    expect(readStoredRunSplit()).toBe(MAX_RUN_SIDE);
  });

  it("falls back to the default for a corrupt value", () => {
    localStorage.setItem("specflow.runSplit", "wide");
    expect(readStoredRunSplit()).toBeNull();
  });
});
