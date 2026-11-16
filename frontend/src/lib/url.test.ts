import { describe, expect, it } from "vitest";
import { parseRoute, routeProject, routeToSearch } from "./url";

describe("route state", () => {
  it("reads a chat session from the query string", () => {
    expect(parseRoute("?run=abc123")).toEqual({ view: "chat", runId: "abc123" });
    expect(parseRoute("?run=abc123&project=%2Frepo")).toEqual({
      view: "chat",
      runId: "abc123",
      project: "/repo",
    });
  });

  it("treats the old flows page as the chat home", () => {
    expect(parseRoute("?view=flows")).toEqual({ view: "home" });
  });

  it("reads the flow editor with and without a reference", () => {
    expect(parseRoute("?view=editor")).toEqual({ view: "flow" });
    expect(parseRoute("?view=editor&edit=standard")).toEqual({
      view: "flow",
      reference: "standard",
    });
    expect(parseRoute("?view=editor&edit=standard&project=%2Frepo")).toEqual({
      view: "flow",
      reference: "standard",
      project: "/repo",
    });
  });

  it("keeps the old history link working as the chat home", () => {
    expect(parseRoute("?view=history")).toEqual({ view: "home" });
  });

  it("defaults to the chat home", () => {
    expect(parseRoute("")).toEqual({ view: "home" });
    expect(parseRoute("?view=unknown")).toEqual({ view: "home" });
  });

  it("prefers a run over a conflicting view", () => {
    expect(parseRoute("?view=history&run=abc")).toEqual({
      view: "chat",
      runId: "abc",
    });
  });

  it("reads the project that roots a run or flow route", () => {
    expect(routeProject({ view: "home" })).toBeUndefined();
    expect(
      routeProject({ view: "chat", runId: "run-1", project: "/repo" }),
    ).toBe("/repo");
    expect(routeProject({ view: "flow", reference: "standard" })).toBeUndefined();
    expect(
      routeProject({ view: "flow", reference: "standard", project: "/repo" }),
    ).toBe("/repo");
  });

  it("round-trips every route", () => {
    const routes = [
      { view: "home" as const },
      { view: "chat" as const, runId: "a b/c" },
      { view: "chat" as const, runId: "run-1", project: "/a b/repo" },
      { view: "flow" as const },
      { view: "flow" as const, reference: "standard-feature" },
      { view: "flow" as const, reference: "standard-feature", project: "/repo" },
    ];
    for (const route of routes) {
      expect(parseRoute(routeToSearch(route))).toEqual(route);
    }
  });
});
