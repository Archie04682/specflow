import type { WorkflowDraft } from "../types/workflowDraft";
import { describe, expect, it } from "vitest";
import {
  addEdge,
  addNode,
  canAddEdge,
  draftSignature,
  edgeKey,
  emptyDraft,
  isValidReference,
  normalizeModel,
  parseTriState,
  replaceEdge,
  REFERENCE_PATTERN,
  referenceFromName,
  removeEdge,
  removeNode,
  triStateValue,
  updateNode,
  updateWorkflowFields,
} from "./model";

describe("editor model", () => {
  it("creates a blank draft with defaults", () => {
    const draft = emptyDraft("demo");
    expect(draft.id).toBe("demo");
    expect(draft.nodes).toEqual([]);
    expect(draft.edges).toEqual([]);
    expect(draft.defaults.harness).toBe("pi");
  });

  it("validates references as bare filenames", () => {
    expect(isValidReference("standard-feature")).toBe(true);
    expect(isValidReference("flow.v2")).toBe(true);
    expect(isValidReference("")).toBe(false);
    expect(isValidReference("../escape")).toBe(false);
    expect(isValidReference("a/b")).toBe(false);
    expect(isValidReference(".hidden")).toBe(false);
    expect(isValidReference("a b")).toBe(false);
  });

  it("adds nodes with unique identifiers", () => {
    const first = addNode(emptyDraft("demo"));
    expect(first.nodeId).toBe("node-1");
    expect(first.draft.nodes).toHaveLength(1);

    const second = addNode(first.draft);
    expect(second.nodeId).toBe("node-2");
    expect(second.draft.nodes.map((node) => node.id)).toEqual([
      "node-1",
      "node-2",
    ]);
  });

  it("updates a node without touching the others", () => {
    const { draft } = addNode(emptyDraft("demo"));
    const next = updateNode(draft, "node-1", { title: "Explore", kind: "command" });
    expect(next.nodes[0]).toMatchObject({ title: "Explore", kind: "command" });
  });

  it("removes a node and every incident edge", () => {
    let draft = emptyDraft("demo");
    draft = addNode(draft).draft;
    draft = addNode(draft).draft;
    draft = addEdge(draft, "node-1", "node-2");
    expect(draft.edges).toHaveLength(1);

    draft = removeNode(draft, "node-2");
    expect(draft.nodes.map((node) => node.id)).toEqual(["node-1"]);
    expect(draft.edges).toEqual([]);
  });

  it("guards edges against duplicates, self-loops, and unknown nodes", () => {
    let draft = emptyDraft("demo");
    draft = addNode(draft).draft;
    draft = addNode(draft).draft;

    expect(canAddEdge(draft, "node-1", "node-2")).toBe(true);
    expect(canAddEdge(draft, "node-1", "node-1")).toBe(false);
    expect(canAddEdge(draft, "node-1", "missing")).toBe(false);

    const withEdge = addEdge(draft, "node-1", "node-2");
    expect(withEdge.edges).toHaveLength(1);
    expect(addEdge(withEdge, "node-1", "node-2")).toBe(withEdge);

    const removed = removeEdge(withEdge, "node-1", "node-2");
    expect(removed.edges).toEqual([]);
  });

  it("normalizes optional model values", () => {
    expect(normalizeModel({ id: "", thinking: "" })).toBeUndefined();
    expect(normalizeModel({ id: "default", thinking: "" })).toEqual({
      id: "default",
    });
    expect(normalizeModel({ id: "m", thinking: "high" })).toEqual({
      id: "m",
      thinking: "high",
    });
  });

  it("maps tri-state controls without losing false", () => {
    expect(triStateValue(undefined)).toBe("");
    expect(triStateValue(false)).toBe("false");
    expect(parseTriState("")).toBeUndefined();
    expect(parseTriState("false")).toBe(false);
    expect(parseTriState("true")).toBe(true);
  });

  it("compares drafts by content", () => {
    const draft = emptyDraft("demo");
    expect(draftSignature(draft)).toBe(draftSignature(emptyDraft("demo")));
    const renamed = updateWorkflowFields(draft, { name: "Changed" });
    expect(draftSignature(renamed)).not.toBe(draftSignature(draft));
  });
});

describe("referenceFromName", () => {
  it("slugifies a workflow name into a file reference", () => {
    expect(referenceFromName("Some new flow")).toBe("some-new-flow");
    expect(referenceFromName("  Add CSV export!  ")).toBe("add-csv-export");
    expect(referenceFromName("Café déjà vu")).toBe("cafe-deja-vu");
    expect(referenceFromName("  --Hello--  ")).toBe("hello");
  });

  it("returns empty when there is nothing usable", () => {
    expect(referenceFromName("")).toBe("");
    expect(referenceFromName("!!!")).toBe("");
    expect(referenceFromName("Привет")).toBe("");
  });

  it("always yields a valid reference or empty", () => {
    const long = referenceFromName("a".repeat(200));
    expect(long).toHaveLength(128);
    expect(referenceFromName("Some new flow")).toMatch(REFERENCE_PATTERN);
    expect(long).toMatch(REFERENCE_PATTERN);
    expect(referenceFromName("!!!")).not.toMatch(REFERENCE_PATTERN);
  });
});

describe("edgeKey and replaceEdge", () => {
  const base: WorkflowDraft = {
    id: "demo",
    version: 1,
    defaults: { harness: "pi" },
    nodes: [
      { id: "a", kind: "agent" },
      { id: "b", kind: "agent" },
      { id: "c", kind: "agent" },
    ],
    edges: [
      { from: "a", to: "b" },
      { from: "a", to: "c" },
    ],
  };

  it("keys an edge by its endpoints", () => {
    expect(edgeKey({ from: "a", to: "b" })).toBe("a->b");
  });

  it("rewires an edge onto a free pair", () => {
    const next = replaceEdge(base, { from: "a", to: "b" }, { from: "b", to: "c" });
    expect(next?.edges).toEqual([
      { from: "b", to: "c" },
      { from: "a", to: "c" },
    ]);
  });

  it("refuses a self-loop, an unknown node, or a duplicate", () => {
    expect(replaceEdge(base, { from: "a", to: "b" }, { from: "c", to: "c" })).toBeNull();
    expect(replaceEdge(base, { from: "a", to: "b" }, { from: "a", to: "missing" })).toBeNull();
    expect(replaceEdge(base, { from: "a", to: "b" }, { from: "a", to: "c" })).toBeNull();
  });
});
